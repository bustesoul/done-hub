package model

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"done-hub/common/config"
	"done-hub/common/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Independent of the channel projection: channel edits must not reset a model's switch.
// Only disabled bindings need a row; absence means enabled.
type ModelChannelOverride struct {
	ChannelID   int    `gorm:"primaryKey;autoIncrement:false"`
	PublicModel string `gorm:"primaryKey;type:varchar(255)"`
}

var modelBindingWriteMu sync.Mutex

func SetModelChannelEnabled(ctx context.Context, channelID int, publicModel string, enabled bool) error {
	modelBindingWriteMu.Lock()
	defer modelBindingWriteMu.Unlock()
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ?", channelID).First(&endpoint).Error; err != nil {
			return err
		}
		var route GatewayModelRoute
		if err := tx.Where("channel_id = ? AND public_model = ?", channelID, publicModel).First(&route).Error; err != nil {
			return err
		}
		if enabled && endpoint.Status != config.ChannelStatusEnabled {
			return errors.New("渠道总开关已关闭，无法启用模型绑定")
		}
		value := ModelChannelOverride{ChannelID: channelID, PublicModel: publicModel}
		if enabled {
			return tx.Where("channel_id = ? AND public_model = ?", channelID, publicModel).Delete(&ModelChannelOverride{}).Error
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&value).Error
	})
	if err != nil {
		return err
	}
	GatewayRoutes.Load()
	return nil
}

func (choice *ChannelChoice) bindingName(name string) string {
	// Match the configured binding, including case-insensitive names and wildcard routes.
	names := splitNonEmpty(choice.Channel.Models)
	for _, n := range names {
		if n == name {
			return n
		}
	}
	if config.ModelNameCaseInsensitiveEnabled {
		for _, n := range names {
			if strings.EqualFold(n, name) {
				return n
			}
		}
	}
	sort.Strings(names)
	var matched string
	if config.ModelNameCaseInsensitiveEnabled {
		matched = utils.GetModelsWithMatchCaseInsensitive(&names, name)
	}
	if matched == "" {
		matched = utils.GetModelsWithMatch(&names, name)
	}
	return matched
}

func (choice *ChannelChoice) modelDisabled(name string) bool {
	return len(choice.DisabledModels) > 0 && choice.DisabledModels[choice.bindingName(name)]
}

func (choice *ChannelChoice) modelWeight(name string) int {
	if route, ok := choice.Routes[name]; ok && route.Weight > 0 {
		return int(route.Weight)
	}
	if route, ok := choice.Routes[choice.bindingName(name)]; ok && route.Weight > 0 {
		return int(route.Weight)
	}
	return int(config.DefaultChannelWeight)
}

func (cc *GatewayRouteIndex) IsModelDisabled(channelID int, modelName string) bool {
	cc.RLock()
	defer cc.RUnlock()
	choice := cc.Channels[channelID]
	return choice != nil && choice.modelDisabled(modelName)
}

type ModelExplorerRoute struct {
	ID        int      `json:"id"`
	Model     string   `json:"model"`
	Name      string   `json:"name"`
	Upstream  string   `json:"upstream"`
	Groups    []string `json:"groups"`
	Profile   string   `json:"profile"`
	Protocols []string `json:"protocols"`
	Priority  int64    `json:"priority"`
	Weight    uint     `json:"weight"`
	Global    bool     `json:"global"`
	Enabled   bool     `json:"enabled"`
	Cool      bool     `json:"cool"`

	LastProbe *ModelProbeResult `json:"lastProbe,omitempty"`
}

type ExplorerModel struct {
	ID     string                `json:"id"`
	Name   string                `json:"name"`
	Family string                `json:"family"`
	Icon   string                `json:"icon"`
	Info   *ModelInfoResponse    `json:"info,omitempty"`
	Routes []*ModelExplorerRoute `json:"routes"`
}

// Public metadata only. Never serialize Channel or GatewayCredential to this endpoint.
func GetExplorerModels(ctx context.Context) ([]*ExplorerModel, error) {
	var endpoints []GatewayEndpoint
	var routes []GatewayModelRoute
	var overrides []ModelChannelOverride
	tx := DB.WithContext(ctx)
	if err := tx.Find(&endpoints).Error; err != nil {
		return nil, err
	}
	if err := tx.Order("public_model, channel_id").Find(&routes).Error; err != nil {
		return nil, err
	}
	if err := tx.Find(&overrides).Error; err != nil {
		return nil, err
	}
	byID := map[int]GatewayEndpoint{}
	disabled := map[ModelChannelOverride]bool{}
	for _, e := range endpoints {
		byID[e.ChannelID] = e
	}
	for _, o := range overrides {
		disabled[o] = true
	}
	models := map[string]*ExplorerModel{}
	for _, r := range routes {
		e, ok := byID[r.ChannelID]
		if !ok {
			continue
		}
		m := models[r.PublicModel]
		if m == nil {
			m = &ExplorerModel{ID: r.PublicModel, Name: r.PublicModel, Icon: DefaultModelIcon, Routes: []*ModelExplorerRoute{}}
			if PricingInstance != nil {
				price := PricingInstance.GetPrice(r.PublicModel)
				m.Info = price.ModelInfo
				if m.Info != nil && m.Info.Name != "" {
					m.Name = m.Info.Name
				}
			}
			models[m.ID] = m
		}
		weight := r.Weight
		if weight == 0 {
			weight = config.DefaultChannelWeight
		}
		var lastProbe *ModelProbeResult
		if r.LastProbe.At > 0 {
			probe := r.LastProbe
			lastProbe = &probe
		}
		m.Routes = append(m.Routes, &ModelExplorerRoute{ID: e.ChannelID, Model: r.PublicModel, Name: e.Name, Upstream: strings.TrimPrefix(r.UpstreamModel, "+"), Groups: splitNonEmpty(e.Group), Profile: e.ProtocolProfileID, Protocols: []string{}, Priority: r.Priority, Weight: weight, Global: e.Status == config.ChannelStatusEnabled, Enabled: !disabled[ModelChannelOverride{e.ChannelID, r.PublicModel}], Cool: GatewayRoutes.IsInCooldown(e.ChannelID, r.PublicModel), LastProbe: lastProbe})
	}
	result := make([]*ExplorerModel, 0, len(models))
	for _, m := range models {
		m.Family = explorerModelFamily(m)
		sort.Slice(m.Routes, func(i, j int) bool {
			a, b := m.Routes[i], m.Routes[j]
			if a.Priority != b.Priority {
				return a.Priority > b.Priority
			}
			return a.ID < b.ID
		})
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// Families describe the model series, never the pricing provider or relay channel.
// Unknown public aliases use upstream names only when every binding agrees.
func explorerModelFamily(m *ExplorerModel) string {
	if family := modelSeries(m.ID); family != "" {
		return family
	}
	family := ""
	for _, r := range m.Routes {
		candidate := modelSeries(r.Upstream)
		if candidate == "" || (family != "" && family != candidate) {
			return "未分类"
		}
		family = candidate
	}
	if family == "" {
		return "未分类"
	}
	return family
}

func modelSeries(name string) string {
	// Aggregators commonly namespace models, e.g. google/gemini-3.8-flash.
	name = strings.ToLower(strings.TrimSpace(name))
	name = name[strings.LastIndex(name, "/")+1:]
	for _, rule := range []struct{ prefix, family string }{
		{"gemini", "Gemini"}, {"gemma", "Gemma"},
		{"claude", "Claude"}, {"gpt", "GPT"}, {"chatgpt", "GPT"},
		{"o1", "OpenAI o系列"}, {"o3", "OpenAI o系列"}, {"o4", "OpenAI o系列"},
		{"deepseek", "DeepSeek"}, {"qwen", "Qwen"}, {"qwq", "QwQ"},
		{"llama", "Llama"}, {"grok", "Grok"}, {"glm", "GLM"},
		{"kimi", "Kimi"}, {"moonshot", "Moonshot"},
		{"mistral", "Mistral"}, {"mixtral", "Mixtral"}, {"codestral", "Codestral"},
		{"doubao", "Doubao"}, {"hunyuan", "Hunyuan"}, {"ernie", "ERNIE"},
		{"phi", "Phi"}, {"yi", "Yi"}, {"command", "Command"},
		{"flux", "FLUX"}, {"dall-e", "DALL·E"}, {"imagen", "Imagen"}, {"veo", "Veo"},
	} {
		if !strings.HasPrefix(name, rule.prefix) {
			continue
		}
		suffix := strings.TrimPrefix(name, rule.prefix)
		if suffix == "" || strings.ContainsRune("-_.:", rune(suffix[0])) || (suffix[0] >= '0' && suffix[0] <= '9') {
			return rule.family
		}
	}
	return ""
}

type ExplorerError struct {
	ID        int    `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Status    int64  `json:"status"`
	Message   string `json:"message"`
}

type ExplorerStats struct {
	ConsumeCount                                          int64           `json:"consumeCount"`
	ErrorCount                                            int64           `json:"errorCount"`
	Quota                                                 int64           `json:"quota"`
	CostQuota                                             int64           `json:"costQuota"`
	CostSampleCount                                       int64           `json:"costSampleCount"`
	FirstResponseMs                                       *float64        `json:"firstResponseMs"`
	FirstSampleCount                                      int64           `json:"firstSampleCount"`
	TPS                                                   *float64        `json:"tps"`
	SpeedSampleCount                                      int64           `json:"speedSampleCount"`
	LatencyMs                                             *float64        `json:"latencyMs"`
	PromptTokens                                          int64           `json:"promptTokens"`
	CompletionTokens                                      int64           `json:"completionTokens"`
	Ratio                                                 *float64        `json:"ratio"`
	Ambiguous                                             int64           `json:"ambiguous"`
	RecentErrors                                          []ExplorerError `json:"recentErrors"`
	firstTotal, speedTokens, latencyTotal, latencySamples int64
	speedMs                                               float64
}

func (s *ExplorerStats) add(l *Log) {
	if l.Type == LogTypeError {
		s.ErrorCount++
		if len(s.RecentErrors) < 3 {
			message, _ := l.Metadata.Data()["error_message"].(string)
			if message == "" {
				message = l.Content
			}
			s.RecentErrors = append(s.RecentErrors, ExplorerError{l.Id, l.CreatedAt, metadataInt64(l.Metadata.Data(), "status_code"), message})
		}
		return
	}
	s.ConsumeCount++
	s.Quota += int64(l.Quota)
	s.CostQuota += int64(l.CostQuota)
	if l.CostQuota > 0 {
		s.CostSampleCount++
	}
	s.PromptTokens += int64(l.PromptTokens)
	s.CompletionTokens += int64(l.CompletionTokens)
	if l.RequestTime > 0 {
		s.latencyTotal += int64(l.RequestTime)
		s.latencySamples++
	}
	first := metadataInt64(l.Metadata.Data(), "first_response")
	if l.IsStream && first > 0 && first <= int64(l.RequestTime) {
		s.FirstSampleCount++
		s.firstTotal += first
	}
	if tokens, duration := LogOutputSpeedSample(l); duration > 0 {
		s.SpeedSampleCount++
		s.speedTokens += tokens
		s.speedMs += duration
	}
}
func (s *ExplorerStats) finish(consumeTracked, errorsTracked bool) {
	avg := func(sum, n int64) *float64 {
		if n == 0 {
			return nil
		}
		v := float64(sum) / float64(n)
		return &v
	}
	s.FirstResponseMs = avg(s.firstTotal, s.FirstSampleCount)
	s.LatencyMs = avg(s.latencyTotal, s.latencySamples)
	if s.speedMs > 0 {
		speed := float64(s.speedTokens) * 1000 / s.speedMs
		s.TPS = &speed
	}
	if consumeTracked && errorsTracked && s.Ambiguous == 0 {
		s.Ratio = avg(s.ConsumeCount*100, s.ConsumeCount+s.ErrorCount)
	}
	if s.RecentErrors == nil {
		s.RecentErrors = []ExplorerError{}
	}
}

type ExplorerHistory struct {
	ByModel        map[string]map[int]*ExplorerStats `json:"byModel"`
	Totals         map[string]*ExplorerStats         `json:"totals"`
	AsOf           int64                             `json:"asOf"`
	ConsumeTracked bool                              `json:"consumeTracked"`
	ErrorsTracked  bool                              `json:"errorsTracked"`
	Unattributed   int64                             `json:"unattributed"`
}

// Use one bounded raw-log pass, not daily statistics or one query per model.
// Historic billing names are attributed only when the current mapping has one candidate.
func GetExplorerHistory(ctx context.Context, models []*ExplorerModel, now time.Time, consumeTracked, errorsTracked bool) (*ExplorerHistory, error) {
	result := &ExplorerHistory{ByModel: map[string]map[int]*ExplorerStats{}, Totals: map[string]*ExplorerStats{}, AsOf: now.Unix(), ConsumeTracked: consumeTracked, ErrorsTracked: errorsTracked}
	byChannel := map[int][]*ModelExplorerRoute{}
	for _, m := range models {
		result.Totals[m.ID] = &ExplorerStats{}
		result.ByModel[m.ID] = map[int]*ExplorerStats{}
		for _, r := range m.Routes {
			result.ByModel[m.ID][r.ID] = &ExplorerStats{}
			byChannel[r.ID] = append(byChannel[r.ID], r)
		}
	}
	tx := DB.WithContext(ctx).Model(&Log{}).Select("id", "created_at", "content", "type", "model_name", "channel_id", "quota", "cost_quota", "prompt_tokens", "completion_tokens", "request_time", "is_stream", "metadata").Where("created_at >= ? AND created_at < ? AND type IN ?", now.Add(-24*time.Hour).Unix(), now.Unix(), []int{LogTypeConsume, LogTypeError})
	err := scanUsageLogs(tx, func(l *Log) error {
		candidates := []*ModelExplorerRoute{}
		routeModel, _ := l.Metadata.Data()["route_model"].(string)
		publicModel, _ := l.Metadata.Data()["public_model"].(string)
		for _, r := range byChannel[l.ChannelId] {
			matches := false
			if routeModel != "" {
				matches = routeModel == r.Model
			} else if publicModel != "" {
				matches = publicModel == r.Model
			} else {
				matches = l.ModelName == r.Model || l.ModelName == r.Upstream
			}
			if matches {
				candidates = append(candidates, r)
			}
		}
		if len(candidates) == 1 {
			r := candidates[0]
			result.ByModel[r.Model][r.ID].add(l)
			result.Totals[r.Model].add(l)
		} else {
			result.Unattributed++
			for _, r := range candidates {
				result.ByModel[r.Model][r.ID].Ambiguous++
				result.Totals[r.Model].Ambiguous++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, s := range result.Totals {
		s.finish(consumeTracked, errorsTracked)
	}
	for _, channels := range result.ByModel {
		for _, s := range channels {
			s.finish(consumeTracked, errorsTracked)
		}
	}
	return result, nil
}
