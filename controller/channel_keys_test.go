package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers"
	"done-hub/providers/base"
	"done-hub/types"
)

func saveTestKeyPool(t *testing.T, channel *model.Channel, keys []model.ChannelKey, strategy string) *model.Channel {
	t.Helper()
	channel.Keys, channel.KeyStrategy, channel.SaveUnverified = keys, strategy, true
	response := callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("保存 Key 失败: %s", response.Body.String())
	}
	for _, key := range keys {
		if key.Secret != "" && strings.Contains(response.Body.String(), key.Secret) {
			t.Fatal("普通保存响应泄露 Key")
		}
	}
	if err := model.UpdateChannelStatusById(channel.Id, 1); err != nil {
		t.Fatal(err)
	}
	loaded, err := model.GetChannelById(channel.Id)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestChannelKeyPoolRoundRobinAndConcurrentSelection(t *testing.T) {
	_, channel := setupEditableProvider(t)
	channel = saveTestKeyPool(t, channel, []model.ChannelKey{
		{Secret: "key-A", Enabled: true}, {Secret: "key-B", Enabled: true}, {Secret: "key-C", Enabled: true},
	}, "")
	if channel.KeyStrategy != model.KeyStrategyRoundRobin {
		t.Fatal("默认策略不是轮询")
	}
	runtime := model.GatewayRoutes.GetChannel(channel.Id)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selected, err := model.SelectGatewayCredential(runtime)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			counts[selected.Key]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	for _, key := range []string{"key-A", "key-B", "key-C"} {
		if counts[key] != 100 {
			t.Fatalf("并发轮询分配不均: %v", counts)
		}
	}
	if runtime.Key != "key-A" || len(runtime.CredentialPool) != 3 {
		t.Fatal("请求修改了共享快照")
	}

	keys := channel.CredentialPool
	removed := keys[0].ID
	keys = keys[1:]
	keys[0].Enabled = false
	channel = saveTestKeyPool(t, channel, keys, model.KeyStrategyRandom)
	runtime = model.GatewayRoutes.GetChannel(channel.Id)
	for i := 0; i < 50; i++ {
		selected, err := model.SelectGatewayCredential(runtime)
		if err != nil || selected.Key != "key-C" {
			t.Fatal("删除或停用的 Key 进入了调度")
		}
	}
	pool, strategy, err := model.LoadGatewayKeyPool(model.DB, channel.Id)
	if err != nil || len(pool) != 2 || strategy != model.KeyStrategyRandom {
		t.Fatal("Key 集合/策略没有持久化")
	}
	for _, key := range pool {
		if key.ID == removed {
			t.Fatal("旧版本 Key 重新进入了可用集合")
		}
	}
	keys = channel.CredentialPool
	keys[1].Enabled = false
	channel = saveTestKeyPool(t, channel, keys, model.KeyStrategyRandom)
	if _, err := model.SelectGatewayCredential(model.GatewayRoutes.GetChannel(channel.Id)); err == nil {
		t.Fatal("所有 Key 停用后仍然可调用")
	}
}

func TestChannelKeysReachActualUpstreamAndRemainPinned(t *testing.T) {
	_, channel := setupEditableProvider(t)
	headers := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"test","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	oldClient := requester.HTTPClient
	requester.HTTPClient = server.Client()
	defer func() { requester.HTTPClient = oldClient }()
	channel.BaseURL = &server.URL
	channel = saveTestKeyPool(t, channel, []model.ChannelKey{{Secret: "transport-A", Enabled: true}, {Secret: "transport-B", Enabled: true}, {Secret: "transport-OFF", Enabled: false}}, model.KeyStrategyRoundRobin)
	runtime := model.GatewayRoutes.GetChannel(channel.Id)
	got := []string{}
	for i := 0; i < 6; i++ {
		provider, err := providers.GetProviderWithError(runtime, base.NewMemoryRequestContext(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)))
		if err != nil {
			t.Fatal(err)
		}
		provider.SetUsage(&types.Usage{})
		_, apiErr := provider.(base.ChatInterface).CreateChatCompletion(&types.ChatCompletionRequest{Model: "gpt-test"})
		if apiErr != nil {
			t.Fatal(apiErr)
		}
		got = append(got, <-headers)
	}
	for i, key := range got {
		if key != "Bearer transport-A" && key != "Bearer transport-B" {
			t.Fatal("上游收到了无效 Key")
		}
		if i > 0 && key == got[i-1] {
			t.Fatal("实际上游请求未按轮询切换")
		}
	}
	pinned, err := model.PinGatewayCredential(runtime, channel.CredentialPool[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		selected, err := model.SelectGatewayCredential(pinned)
		if err != nil || selected.Key != "transport-B" {
			t.Fatal("异步任务绑定的 Key 被重新分配")
		}
	}
	if channel.CredentialPool[0].Secret == pinned.Key || channel.CredentialCacheKey("oauth") == pinned.CredentialCacheKey("oauth") {
		t.Fatal("不同 Key 使用了相同的 token 缓存")
	}
	if err := pinned.SaveRefreshedCredential("refreshed-B"); err != nil {
		t.Fatal(err)
	}
	pool, _, err := model.LoadGatewayKeyPool(model.DB, channel.Id)
	if err != nil || len(pool) != 3 || pool[0].Secret != "transport-A" || pool[1].Secret != "refreshed-B" {
		t.Fatal("OAuth 刷新覆盖了其他 Key")
	}
}

func TestMultiKeyCreateProbeAndValidation(t *testing.T) {
	db, _ := setupEditableProvider(t)
	jsonCredential := "{\n  \"access_token\": \"json-token\"\n}"
	draft := model.Channel{Type: 1, ProtocolProfileID: "openai-chat-completions", Name: "multi-created", Models: "gpt-test", TestModel: "gpt-test", Group: "default", SaveUnverified: true,
		Keys: []model.ChannelKey{{Secret: jsonCredential, Enabled: true}, {Secret: "second-token", Enabled: true}}, KeyStrategy: model.KeyStrategyRandom}
	response := callProviderHandler(t, 0, http.MethodPost, draft, AddProviderConnection)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("创建失败: %s", response.Body.String())
	}
	var channels []model.Channel
	if err := db.Where("name = ?", draft.Name).Find(&channels).Error; err != nil || len(channels) != 1 {
		t.Fatal("多 Key 被拆成了多个渠道")
	}
	pool, strategy, err := model.LoadGatewayKeyPool(db, channels[0].Id)
	if err != nil || len(pool) != 2 || pool[0].Secret != jsonCredential || strategy != model.KeyStrategyRandom {
		t.Fatal("多行 JSON 凭据未被完整保存")
	}
	draft.Keys[0].ID = pool[0].ID
	token, err := issueProviderValidationToken(&draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.ValidationToken = token
	draft.Keys[1].Secret = "changed-after-probe"
	if verifyProviderValidationToken(&draft) == nil {
		t.Fatal("探测令牌没有绑定完整 Key 集合")
	}
	loaded, err := model.GetChannelById(channels[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Keys = pool
	loaded.Keys[0].ID = 999999
	loaded.Name = "must-rollback"
	loaded.SaveUnverified = true
	response = callProviderHandler(t, loaded.Id, http.MethodPatch, loaded, UpdateChannel)
	if strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatal("越权/失效的凭据 ID 被接受")
	}
	stored, err := model.GetChannelById(loaded.Id)
	if err != nil || stored.Name != draft.Name {
		t.Fatal("无效 Key 保存没有回滚渠道配置")
	}
	response = callProviderHandler(t, loaded.Id, http.MethodGet, nil, GetProviderConnectionKeys)
	var result struct {
		Data struct {
			Keys []model.ChannelKey `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Data.Keys) != 2 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Key 编辑接口异常")
	}
}

func TestMultiKeyProbeChecksEnabledKeysAndBindsSave(t *testing.T) {
	_, channel := setupEditableProvider(t)
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seen[request.Header.Get("Authorization")]++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"test","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	oldClient := requester.HTTPClient
	requester.HTTPClient = server.Client()
	defer func() { requester.HTTPClient = oldClient }()
	channel.BaseURL = &server.URL
	channel.Keys = []model.ChannelKey{{Secret: "disabled-key", Enabled: false}, {Secret: "probe-A", Enabled: true}, {Secret: "probe-B", Enabled: true}}
	response := callProviderHandler(t, channel.Id, http.MethodPost, channel, ProbeProviderConnection)
	if response.Code != http.StatusOK || seen["Bearer probe-A"] == 0 || seen["Bearer probe-B"] == 0 || seen["Bearer disabled-key"] != 0 {
		t.Fatalf("多 Key 探测没有正确覆盖已启用的 Key: %s", response.Body.String())
	}
	var result struct {
		Data struct {
			Token string `json:"validation_token"`
			Count int    `json:"tested_connections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Data.Count != 2 {
		t.Fatal("多 Key 探测结果不正确")
	}
	channel.ValidationToken = result.Data.Token
	response = callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("多 Key 探测令牌不能保存: %s", response.Body.String())
	}
	pool, _, err := model.LoadGatewayKeyPool(model.DB, channel.Id)
	if err != nil || len(pool) != 3 {
		t.Fatal("探测后保存的 Key 集合不正确")
	}
	primary, plaintext, err := model.LoadActiveGatewayCredentialSecret(model.DB, channel.Id)
	if err != nil || plaintext != "probe-A" || primary.TestedAt == nil {
		t.Fatal("主凭据没有指向已启用且已探测的 Key")
	}
}

func TestMultiKeyFailuresDoNotCoolDownOtherKeys(t *testing.T) {
	_, channel := setupEditableProvider(t)
	channel = saveTestKeyPool(t, channel, []model.ChannelKey{{Secret: "failed-key", Enabled: true}, {Secret: "healthy-key", Enabled: true}}, model.KeyStrategyRoundRobin)
	if !model.CooldownGatewayCredential(channel.Id, channel.CredentialPool[0].ID, 30) {
		t.Fatal("未进入 Key 级冷却")
	}
	runtime := model.GatewayRoutes.GetChannel(channel.Id)
	for i := 0; i < 5; i++ {
		selected, err := model.SelectGatewayCredential(runtime)
		if err != nil || selected.Key != "healthy-key" {
			t.Fatal("失败 Key 影响了其他 Key")
		}
	}
	if err := model.DisableGatewayCredential(channel.Id, channel.CredentialPool[0].ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := model.GetChannelById(channel.Id)
	if err != nil || loaded.Status != 1 || loaded.CredentialPool[0].Enabled || !loaded.CredentialPool[1].Enabled {
		t.Fatal("单 Key 禁用影响了整个渠道")
	}
}

func TestExistingAsyncTasksKeepOriginalCredentialAfterUpgrade(t *testing.T) {
	db, channel := setupEditableProvider(t)
	if err := db.AutoMigrate(&model.Task{}, &model.Midjourney{}); err != nil {
		t.Fatal(err)
	}
	task := model.Task{TaskID: "before-upgrade", ChannelId: channel.Id}
	mj := model.Midjourney{MjId: "before-upgrade", ChannelId: channel.Id}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&mj).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.BackfillTaskCredentialReferences(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&mj, mj.Id).Error; err != nil {
		t.Fatal(err)
	}
	if task.CredentialID != channel.CredentialID || mj.CredentialID != channel.CredentialID || task.CredentialID == 0 {
		t.Fatal("升级时没有固定原有任务的 Key")
	}
	channel = saveTestKeyPool(t, channel, []model.ChannelKey{{Secret: "new-account-key", Enabled: true}}, model.KeyStrategyRoundRobin)
	if err := model.BackfillTaskCredentialReferences(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	pinned, err := model.PinGatewayCredential(channel, task.CredentialID)
	if err != nil || pinned.Key != "original-secret" {
		t.Fatal("任务的原 Key 被后续保存覆盖")
	}
}

func TestKeySwitchAndStrategySaveKeepChannelEnabled(t *testing.T) {
	_, channel := setupEditableProvider(t)
	channel = saveTestKeyPool(t, channel, []model.ChannelKey{{Secret: "existing-A", Enabled: true}, {Secret: "existing-B", Enabled: true}}, model.KeyStrategyRoundRobin)
	channel.Keys = channel.CredentialPool
	channel.Keys[0].Enabled = false
	channel.KeyStrategy = model.KeyStrategyRandom
	channel.SaveUnverified = true
	response := callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatal("Key 开关保存失败")
	}
	loaded, err := model.GetChannelById(channel.Id)
	if err != nil || loaded.Status != 1 || loaded.KeyStrategy != model.KeyStrategyRandom || loaded.CredentialPool[0].Enabled {
		t.Fatal("调整开关或策略意外停用了整个渠道")
	}
}
