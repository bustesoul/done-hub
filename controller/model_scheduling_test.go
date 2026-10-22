package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/secret"
	"done-hub/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestModelBindingProbeUsesMappingWithoutMutatingHealth(t *testing.T) {
	var fail atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1beta/models/upstream-model:generateContent" {
			t.Errorf("wrong model/path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			w.WriteHeader(503)
			w.Write([]byte(`{"error":{"code":503,"message":"test unavailable","status":"UNAVAILABLE"}}`))
			return
		}
		w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"totalTokenCount":6}}`))
	}))
	defer server.Close()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Channel{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := model.AutoMigrateGatewayResources(db); err != nil {
		t.Fatal(err)
	}
	oldDB, oldLogger, oldClient, oldKey := model.DB, logger.Logger, requester.HTTPClient, viper.GetString("gateway_secret_key")
	model.DB, logger.Logger, requester.HTTPClient = db, zap.NewNop(), server.Client()
	viper.Set("gateway_secret_key", "probe-binding-test")
	defer func() {
		model.DB, logger.Logger, requester.HTTPClient = oldDB, oldLogger, oldClient
		viper.Set("gateway_secret_key", oldKey)
		sql, _ := db.DB()
		sql.Close()
	}()
	mapping := `{"client-model":"upstream-model"}`
	channel := model.Channel{Type: config.ChannelTypeGemini, ProtocolProfileID: "google-gemini", Key: "test-key", BaseURL: &server.URL, ModelMapping: &mapping, Models: "client-model", Group: "default", Status: config.ChannelStatusManuallyDisabled, ResponseTime: 987, TestTime: 123}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	cipher, _ := secret.New("probe-binding-test")
	if err := model.BackfillGatewayResources(db, cipher); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		// Read persisted state rather than trusting the response's success flag.
		var ch []model.Channel
		var h []model.GatewayHealthState
		var c []model.GatewayCredential
		var o []model.ModelChannelOverride
		var l []model.Log
		db.Find(&ch)
		db.Find(&h)
		db.Find(&c)
		db.Find(&o)
		db.Find(&l)
		b, _ := json.Marshal([]any{ch, h, c, o, l})
		return string(b)
	}
	before := snapshot()
	for _, failed := range []bool{false, true} {
		fail.Store(failed)
		body, _ := json.Marshal(map[string]any{"channel_id": channel.Id, "model": "client-model"})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/admin/models/probe", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		ProbeModelExplorerBinding(c)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["success"] != !failed {
			t.Fatalf("probe failed=%v response=%s", failed, w.Body.String())
		}
		if !reflect.DeepEqual(before, snapshot()) {
			t.Fatal("manual probe changed persisted channel health/credentials/logs/switches")
		}
		catalog, err := model.GetExplorerModels(c.Request.Context())
		if err != nil || len(catalog) != 1 || len(catalog[0].Routes) != 1 {
			t.Fatal("cannot reload probe from catalog", catalog, err)
		}
		probe := catalog[0].Routes[0].LastProbe
		if probe == nil || probe.At == 0 || probe.Success != !failed || probe.Message == "" {
			t.Fatal("completed probe not persisted", probe)
		}
		responseProbe := result["data"].(map[string]any)["last_probe"]
		wantJSON, _ := json.Marshal(probe)
		gotJSON, _ := json.Marshal(responseProbe)
		var want, got map[string]any
		json.Unmarshal(wantJSON, &want)
		json.Unmarshal(gotJSON, &got)
		if !reflect.DeepEqual(want, got) {
			t.Fatal("response differs from persisted result", want, got)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("probe issued extra requests", calls.Load())
	}
	for _, name := range []string{"missing-model", "wild-*"} {
		body, _ := json.Marshal(map[string]any{"channel_id": channel.Id, "model": name})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/admin/models/probe", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		ProbeModelExplorerBinding(c)
		if w.Code < 400 {
			t.Fatal("invalid binding accepted", name)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("invalid binding reached upstream")
	}
}

func TestModelSchedulingInputValidation(t *testing.T) {
	for _, body := range []string{
		`{"channel_id":1,"model":"x","weight":1}`,
		`{"channel_id":1,"model":"x","priority":0,"weight":0}`,
		`{"channel_id":1,"model":"x","priority":0,"weight":1.5}`,
		`{"channel_id":1,"model":"x","priority":0,"weight":1000001}`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PATCH", "/api/admin/models/scheduling", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		SetModelExplorerScheduling(c)
		if w.Code != 400 {
			t.Fatal("invalid input accepted", body, w.Code)
		}
	}
}
