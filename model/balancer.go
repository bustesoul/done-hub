package model

import (
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/utils"
	"done-hub/internal/gateway/requeststate"
	gatewayselection "done-hub/internal/gateway/selection"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 错误消息常量
const (
	ErrNoAvailableChannelForModel        = "当前分组 %s 下对于模型 %s 无可用渠道"
	ErrModelNotFound                     = "model not found"
	ErrChannelNotFound                   = "channel not found"
	ErrModelNotFoundInGroup              = "model not found in group"
	ErrNoChannelsAvailable               = "no channels available for model"
	ErrNoAvailableChannelsAfterFiltering = "no available channels after filtering"
	ErrDatabaseConsistencyBroken         = "数据库一致性已被破坏，请联系管理员"
	ErrInvalidChannelId                  = "无效的渠道 Id"
	ErrChannelDisabled                   = "该渠道已被禁用"
)

// runtime 错误 sentinel：表示"模型有配但渠道暂时不可用"（列表空、全冷却/过滤、指定 id 失效或被禁），
// 与"配置层就没配"的错误区分开。必须是 sentinel —— fetchChannelByModel 会 wrap，上层用 errors.Is 跨层判定。
var (
	ErrNoChannelsAvailableSentinel               = errors.New(ErrNoChannelsAvailable)
	ErrNoAvailableChannelsAfterFilteringSentinel = errors.New(ErrNoAvailableChannelsAfterFiltering)
	ErrInvalidChannelIdSentinel                  = errors.New(ErrInvalidChannelId)
	ErrChannelDisabledSentinel                   = errors.New(ErrChannelDisabled)
)

type ChannelChoice struct {
	Channel       *Channel
	CooldownsTime int64
	Disable       bool
}

type GatewayRouteIndex struct {
	sync.RWMutex
	Channels  map[int]*ChannelChoice
	Rule      map[string]map[string][][]int // group -> model -> priority -> channelIds
	Match     []string
	Cooldowns sync.Map

	ModelGroup map[string]map[string]bool
}

type ChannelsFilterFunc func(channelId int, choice *ChannelChoice) bool

func FilterChannelId(skipChannelIds []int) ChannelsFilterFunc {
	return func(channelId int, _ *ChannelChoice) bool {
		return utils.Contains(channelId, skipChannelIds)
	}
}

func FilterChannelTypes(channelTypes []int) ChannelsFilterFunc {
	return func(_ int, choice *ChannelChoice) bool {
		return !utils.Contains(choice.Channel.Type, channelTypes)
	}
}

func FilterOnlyChat() ChannelsFilterFunc {
	return func(channelId int, choice *ChannelChoice) bool {
		return choice.Channel.OnlyChat
	}
}

func FilterDisabledStream(modelName string) ChannelsFilterFunc {
	return func(_ int, choice *ChannelChoice) bool {
		return !choice.Channel.AllowStream(modelName)
	}
}

func init() {
	// 每5分钟清理一次过期的冷却时间，加快内存回收
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			GatewayRoutes.CleanupExpiredCooldowns()
		}
	}()
}

func (cc *GatewayRouteIndex) SetCooldowns(channelId int, modelName string) bool {
	return cc.SetCooldownsWithDuration(channelId, modelName, int64(config.RetryCooldownSeconds))
}

// SetCooldownsWithDuration 设置指定渠道和模型的冷却时间（支持自定义冻结时长）
func (cc *GatewayRouteIndex) SetCooldownsWithDuration(channelId int, modelName string, durationSeconds int64) bool {
	if channelId == 0 || modelName == "" || durationSeconds == 0 {
		return false
	}

	key := fmt.Sprintf("%d:%s", channelId, modelName)
	nowTime := time.Now().Unix()
	newCooldownTime := nowTime + durationSeconds

	// 使用LoadOrStore的原子性，避免竞态条件
	actualValue, loaded := cc.Cooldowns.LoadOrStore(key, newCooldownTime)

	if loaded {
		// key已存在，检查是否仍在冷却期内
		existingCooldownTime := actualValue.(int64)
		if nowTime < existingCooldownTime {
			// 仍在冷却期内，无需重新设置
			return true
		}
		// 冷却期已过，尝试更新为新的冷却时间
		// 如果CompareAndSwap失败，说明其他线程已经更新了，这也是可以接受的
		cc.Cooldowns.CompareAndSwap(key, existingCooldownTime, newCooldownTime)
	}

	return true
}

func (cc *GatewayRouteIndex) IsInCooldown(channelId int, modelName string) bool {
	if channelId == 0 || modelName == "" {
		return false
	}

	key := fmt.Sprintf("%d:%s", channelId, modelName)
	nowTime := time.Now().Unix()

	cooldownTime, exists := cc.Cooldowns.Load(key)
	if !exists {
		return false
	}

	// 直接返回冷却状态，不进行任何清理操作
	return nowTime < cooldownTime.(int64)
}

func (cc *GatewayRouteIndex) CleanupExpiredCooldowns() {
	now := time.Now().Unix()
	cc.Cooldowns.Range(func(key, value interface{}) bool {
		if now >= value.(int64) {
			cc.Cooldowns.Delete(key)
		}
		return true
	})
}

// ClearChannelCooldowns 清除指定渠道的所有冻结缓存
func (cc *GatewayRouteIndex) ClearChannelCooldowns(channelId int) {
	prefix := fmt.Sprintf("%d:", channelId)
	cc.Cooldowns.Range(func(key, value interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			cc.Cooldowns.Delete(key)
		}
		return true
	})
}

func (cc *GatewayRouteIndex) Disable(channelId int) {
	cc.Lock()
	defer cc.Unlock()
	if _, ok := cc.Channels[channelId]; !ok {
		return
	}

	cc.Channels[channelId].Disable = true
}

func (cc *GatewayRouteIndex) Enable(channelId int) {
	cc.Lock()
	defer cc.Unlock()
	if _, ok := cc.Channels[channelId]; !ok {
		return
	}

	cc.Channels[channelId].Disable = false
}

func (cc *GatewayRouteIndex) ChangeStatus(channelId int, status bool) {
	if status {
		cc.Enable(channelId)
	} else {
		cc.Disable(channelId)
	}
}

func (cc *GatewayRouteIndex) balancer(channelIds []int, choices map[int]*ChannelChoice, filters []ChannelsFilterFunc, modelName string, ginContext interface{}) *Channel {
	// 候选集已由调用方限定在同一优先级层。先完成健康、冷却和请求能力过滤，
	// 再决定使用无状态亲和或原有权重随机，亲和不能绕过任何过滤条件。
	totalWeight := 0

	validChannels := make([]*ChannelChoice, 0, len(channelIds))
	for _, channelId := range channelIds {
		choice, ok := choices[channelId]
		if !ok || choice.Disable {
			continue
		}

		if cc.IsInCooldown(channelId, modelName) {
			continue
		}

		isSkip := false
		for _, filter := range filters {
			if filter(channelId, choice) {
				isSkip = true
				break
			}
		}
		if isSkip {
			continue
		}

		weight := int(*choice.Channel.Weight)
		totalWeight += weight
		validChannels = append(validChannels, choice)
	}

	if len(validChannels) == 0 {
		return nil
	}

	if len(validChannels) == 1 {
		return validChannels[0].Channel
	}

	if selected := selectAffinityChannel(validChannels, modelName, ginContext); selected != nil {
		return selected
	}

	choiceWeight := rand.Intn(totalWeight)
	for _, choice := range validChannels {
		weight := int(*choice.Channel.Weight)
		choiceWeight -= weight
		if choiceWeight < 0 {
			return choice.Channel
		}
	}

	return nil
}

func selectAffinityChannel(candidates []*ChannelChoice, modelName string, ginContext interface{}) *Channel {
	gc, ok := ginContext.(*gin.Context)
	if !ok || gc == nil || gc.Request == nil || len(candidates) == 0 {
		return nil
	}
	affinityCandidates := make([]gatewayselection.Candidate, 0, len(candidates))
	fallbackCandidates := make([]gatewayselection.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.Channel == nil {
			continue
		}
		weight := 1
		if candidate.Channel.Weight != nil && *candidate.Channel.Weight > 0 {
			weight = int(*candidate.Channel.Weight)
		}
		weighted := gatewayselection.Candidate{
			EndpointID: candidate.Channel.Id,
			Weight:     weight,
		}
		fallbackCandidates = append(fallbackCandidates, weighted)
		if candidate.Channel.AffinityEnabled {
			affinityCandidates = append(affinityCandidates, weighted)
		}
	}
	state := requeststate.From(gc.Request.Context())
	if state == nil || state.InboundProtocol() == "" {
		return nil
	}
	const affinityActiveKey = "gateway_affinity_active"
	if len(affinityCandidates) > 0 {
		state.Set(affinityActiveKey, true)
	} else if !state.GetBool(affinityActiveKey) {
		return nil
	} else {
		// Every affinity-enabled endpoint was filtered or excluded during retry;
		// keep deterministic failover across the remaining same-priority pool.
		affinityCandidates = fallbackCandidates
	}
	var body []byte
	if raw, exists := gc.Get(config.GinRequestBodyKey); exists {
		body, _ = raw.([]byte)
	}
	key, ok := gatewayselection.BuildAffinityKey(gatewayselection.AffinityInput{
		Protocol: state.InboundProtocol(),
		Headers:  gc.Request.Header,
		Body:     body,
		UserID:   gc.GetInt("id"),
		TokenID:  gc.GetInt("token_id"),
		Group:    gc.GetString("token_group"),
		Model:    modelName,
	})
	if !ok {
		return nil
	}
	selectedID, ok := gatewayselection.PickWeightedRendezvous(key, affinityCandidates)
	if !ok {
		return nil
	}
	for _, candidate := range candidates {
		if candidate != nil && candidate.Channel != nil && candidate.Channel.Id == selectedID {
			state.Set("gateway_affinity_source", key.Source)
			state.Set("gateway_affinity_key_fingerprint", key.Digest[:8])
			return candidate.Channel
		}
	}
	return nil
}

// GetMatchedModelName 获取匹配到的实际模型名称
func (cc *GatewayRouteIndex) GetMatchedModelName(group, modelName string) (string, error) {
	cc.RLock()
	defer cc.RUnlock()
	if _, ok := cc.Rule[group]; !ok {
		return "", fmt.Errorf(ErrNoAvailableChannelForModel, GlobalUserGroupRatio.GetDisplayName(group), modelName)
	}

	// 如果直接匹配到了，返回原始模型名称
	if _, ok := cc.Rule[group][modelName]; ok {
		return modelName, nil
	}

	var matchModel string

	if config.ModelNameCaseInsensitiveEnabled {
		// 1. 先尝试精确的大小写不敏感匹配
		modelNameLower := strings.ToLower(modelName)
		for existingModel := range cc.Rule[group] {
			if strings.ToLower(existingModel) == modelNameLower {
				matchModel = existingModel
				break
			}
		}
		// 2. 如果没找到，再尝试通配符的大小写不敏感匹配
		if matchModel == "" {
			matchModel = utils.GetModelsWithMatchCaseInsensitive(&cc.Match, modelName)
		}
	}

	// 3. 如果还是没找到，使用原始匹配作为后备
	if matchModel == "" {
		matchModel = utils.GetModelsWithMatch(&cc.Match, modelName)
	}

	if matchModel == "" {
		message := fmt.Sprintf(ErrNoAvailableChannelForModel, GlobalUserGroupRatio.GetDisplayName(group), modelName)
		return "", errors.New(message)
	}

	return matchModel, nil
}

func (cc *GatewayRouteIndex) Next(group, modelName string, filters ...ChannelsFilterFunc) (*Channel, error) {
	cc.RLock()
	if _, ok := cc.Rule[group]; !ok {
		cc.RUnlock()
		return nil, fmt.Errorf(ErrNoAvailableChannelForModel, GlobalUserGroupRatio.GetDisplayName(group), modelName)
	}

	channelsPriority, ok := cc.Rule[group][modelName]
	if !ok {
		var matchModel string

		if config.ModelNameCaseInsensitiveEnabled {
			// 1. 先尝试精确的大小写不敏感匹配
			modelNameLower := strings.ToLower(modelName)
			for existingModel := range cc.Rule[group] {
				if strings.ToLower(existingModel) == modelNameLower {
					matchModel = existingModel
					break
				}
			}
			// 2. 如果没找到，再尝试通配符的大小写不敏感匹配
			if matchModel == "" {
				matchModel = utils.GetModelsWithMatchCaseInsensitive(&cc.Match, modelName)
			}
		}

		// 3. 如果还是没找到，使用原始匹配作为后备
		if matchModel == "" {
			matchModel = utils.GetModelsWithMatch(&cc.Match, modelName)
		}

		channelsPriority, ok = cc.Rule[group][matchModel]
		if !ok {
			cc.RUnlock()
			return nil, errors.New(ErrModelNotFound)
		}
	}

	if len(channelsPriority) == 0 {
		cc.RUnlock()
		return nil, errors.New(ErrChannelNotFound)
	}
	channelsPriority, choices := cc.snapshotCandidatesLocked(channelsPriority)
	cc.RUnlock()

	for _, priority := range channelsPriority {
		channel := cc.balancer(priority, choices, filters, modelName, nil)
		if channel != nil {
			return channel, nil
		}
	}

	return nil, errors.New(ErrChannelNotFound)
}

// NextByValidatedModel 使用已经验证过的模型名称获取渠道，跳过模型匹配逻辑
// ginContext 提供协议请求信息，用于可选的无状态渠道亲和选路。
func (cc *GatewayRouteIndex) NextByValidatedModel(group, validatedModelName string, ginContext interface{}, filters ...ChannelsFilterFunc) (*Channel, error) {
	cc.RLock()

	if _, ok := cc.Rule[group]; !ok {
		cc.RUnlock()
		return nil, fmt.Errorf(ErrNoAvailableChannelForModel, GlobalUserGroupRatio.GetDisplayName(group), validatedModelName)
	}

	channelsPriority, ok := cc.Rule[group][validatedModelName]
	if !ok {
		cc.RUnlock()
		return nil, errors.New(ErrModelNotFoundInGroup)
	}

	if len(channelsPriority) == 0 {
		cc.RUnlock()
		return nil, ErrNoChannelsAvailableSentinel
	}
	channelsPriority, choices := cc.snapshotCandidatesLocked(channelsPriority)
	cc.RUnlock()

	for _, priority := range channelsPriority {
		channel := cc.balancer(priority, choices, filters, validatedModelName, ginContext)
		if channel != nil {
			return channel, nil
		}
	}

	return nil, ErrNoAvailableChannelsAfterFilteringSentinel
}

func (cc *GatewayRouteIndex) GetGroupModels(group string) ([]string, error) {
	cc.RLock()
	defer cc.RUnlock()

	if _, ok := cc.Rule[group]; !ok {
		return nil, fmt.Errorf(ErrNoAvailableChannelForModel, GlobalUserGroupRatio.GetDisplayName(group), "*")
	}

	models := make([]string, 0, len(cc.Rule[group]))
	for model := range cc.Rule[group] {
		models = append(models, model)
	}

	return models, nil
}

func (cc *GatewayRouteIndex) GetModelsGroups() map[string]map[string]bool {
	cc.RLock()
	defer cc.RUnlock()

	result := make(map[string]map[string]bool, len(cc.ModelGroup))
	for modelName, groups := range cc.ModelGroup {
		groupCopy := make(map[string]bool, len(groups))
		for group, enabled := range groups {
			groupCopy[group] = enabled
		}
		result[modelName] = groupCopy
	}
	return result
}

func (cc *GatewayRouteIndex) snapshotCandidatesLocked(priorities [][]int) ([][]int, map[int]*ChannelChoice) {
	priorityCopy := make([][]int, len(priorities))
	choices := make(map[int]*ChannelChoice)
	for index, channelIDs := range priorities {
		priorityCopy[index] = append([]int(nil), channelIDs...)
		for _, channelID := range channelIDs {
			if choice, exists := cc.Channels[channelID]; exists && choice != nil {
				choiceCopy := *choice
				choices[channelID] = &choiceCopy
			}
		}
	}
	return priorityCopy, choices
}

func (cc *GatewayRouteIndex) GetChannel(channelId int) *Channel {
	cc.RLock()
	defer cc.RUnlock()

	if choice, ok := cc.Channels[channelId]; ok {
		return choice.Channel
	}

	return nil
}

// CountAvailableChannels 计算指定分组和模型的可用渠道数量（排除禁用、冷却和过滤的渠道）
func (cc *GatewayRouteIndex) CountAvailableChannels(group, modelName string, filters ...ChannelsFilterFunc) int {
	cc.RLock()
	defer cc.RUnlock()

	if _, ok := cc.Rule[group]; !ok {
		return 0
	}

	channelsPriority, ok := cc.Rule[group][modelName]
	if !ok {
		return 0
	}

	if len(channelsPriority) == 0 {
		return 0
	}

	totalAvailable := 0
	for _, priority := range channelsPriority {
		totalAvailable += cc.countValidChannels(priority, filters, modelName)
	}

	return totalAvailable
}

// countValidChannels 计算指定渠道列表中的可用渠道数量
// 与balancer方法使用相同的过滤逻辑
func (cc *GatewayRouteIndex) countValidChannels(channelIds []int, filters []ChannelsFilterFunc, modelName string) int {
	count := 0
	for _, channelId := range channelIds {
		choice, ok := cc.Channels[channelId]
		if !ok || choice.Disable {
			continue
		}

		if cc.IsInCooldown(channelId, modelName) {
			continue
		}

		isSkip := false
		for _, filter := range filters {
			if filter(channelId, choice) {
				isSkip = true
				break
			}
		}
		if isSkip {
			continue
		}

		count++
	}
	return count
}

var GatewayRoutes = GatewayRouteIndex{}

func (cc *GatewayRouteIndex) Load() {
	channels, err := LoadGatewayChannelSnapshots(DB)
	if err != nil {
		logger.SysError("gateway channel snapshot load failed: " + err.Error())
		return
	}

	newGroup := make(map[string]map[string][][]int)
	newChannels := make(map[int]*ChannelChoice)
	newMatch := make(map[string]bool)
	newModelGroup := make(map[string]map[string]bool)

	type groupModelKey struct {
		group string
		model string
	}
	channelGroups := make(map[groupModelKey]map[int64][]int)

	// 处理每个channel
	for _, channel := range channels {
		channel.SetProxy()
		if *channel.Weight == 0 {
			channel.Weight = &config.DefaultChannelWeight
		}
		newChannels[channel.Id] = &ChannelChoice{
			Channel:       channel,
			CooldownsTime: 0,
			Disable:       false,
		}

		// 处理groups和models
		groups := strings.Split(channel.Group, ",")
		models := strings.Split(channel.Models, ",")

		for _, group := range groups {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}

			for _, model := range models {
				model = strings.TrimSpace(model)
				if model == "" {
					continue
				}

				key := groupModelKey{group: group, model: model}
				if _, ok := channelGroups[key]; !ok {
					channelGroups[key] = make(map[int64][]int)
				}

				// 按priority分组存储channelId
				priority := *channel.Priority
				channelGroups[key][priority] = append(channelGroups[key][priority], channel.Id)

				// 处理通配符模型
				if strings.HasSuffix(model, "*") {
					newMatch[model] = true
				}

				// 初始化ModelGroup
				if _, ok := newModelGroup[model]; !ok {
					newModelGroup[model] = make(map[string]bool)
				}
				newModelGroup[model][group] = true
			}
		}
	}

	// 构建最终的newGroup结构
	for key, priorityMap := range channelGroups {
		// 初始化group和model的map
		if _, ok := newGroup[key.group]; !ok {
			newGroup[key.group] = make(map[string][][]int)
		}

		// 获取所有优先级并排序（从大到小）
		var priorities []int64
		for priority := range priorityMap {
			priorities = append(priorities, priority)
		}
		sort.Slice(priorities, func(i, j int) bool {
			return priorities[i] > priorities[j]
		})

		// 按优先级顺序构建[][]int
		var channelsList [][]int
		for _, priority := range priorities {
			channelsList = append(channelsList, priorityMap[priority])
		}

		newGroup[key.group][key.model] = channelsList
	}

	// 构建newMatchList
	newMatchList := make([]string, 0, len(newMatch))
	for match := range newMatch {
		newMatchList = append(newMatchList, match)
	}
	sort.Strings(newMatchList)

	// 更新GatewayRouteIndex
	cc.Lock()
	cc.Rule = newGroup
	cc.Channels = newChannels
	cc.Match = newMatchList
	cc.ModelGroup = newModelGroup
	cc.Unlock()
	logger.SysLog("channels Load success")
}
