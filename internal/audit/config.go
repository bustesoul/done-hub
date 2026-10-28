// Package audit observes existing reads. It never owns routing, upstream reads,
// retries or billing, and never waits for file I/O on a request goroutine.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync/atomic"
)

const OptionKey = "AuditLogConfig"

type Config struct {
	ChannelIDs     []int `json:"channel_ids"`
	TokenIDs       []int `json:"token_ids"`
	ArchiveEnabled bool  `json:"archive_enabled"`
}

func ParseConfig(value string) (Config, error) {
	c := Config{ChannelIDs: []int{}, TokenIDs: []int{}}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("无效的审计配置: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, fmt.Errorf("审计配置只能包含一个 JSON 对象")
	}
	if strings.TrimSpace(value) == "null" {
		return Config{}, fmt.Errorf("审计配置必须是 JSON 对象")
	}
	for _, ids := range [][]int{c.ChannelIDs, c.TokenIDs} {
		if len(ids) > 10000 {
			return Config{}, fmt.Errorf("审计筛选项不能超过 10000 个")
		}
		for _, id := range ids {
			if id <= 0 {
				return Config{}, fmt.Errorf("审计筛选 ID 必须为正整数")
			}
		}
	}
	c.ChannelIDs = normalizeIDs(c.ChannelIDs)
	c.TokenIDs = normalizeIDs(c.TokenIDs)
	return c, nil
}

func normalizeIDs(ids []int) []int {
	ids = append([]int{}, ids...)
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (c Config) Enabled() bool { return len(c.ChannelIDs)+len(c.TokenIDs) > 0 }

func (c Config) matchesToken(id int) bool {
	return id > 0 && (len(c.TokenIDs) == 0 || slices.Contains(c.TokenIDs, id))
}

func (c Config) Matches(channelID, tokenID int) bool {
	return c.Enabled() && channelID > 0 && c.matchesToken(tokenID) &&
		(len(c.ChannelIDs) == 0 || slices.Contains(c.ChannelIDs, channelID))
}

type Service struct {
	config      atomic.Pointer[Config]
	recorder    atomic.Pointer[Recorder]
	configError atomic.Pointer[string]
}

var Default = NewService()

func NewService() *Service {
	s := &Service{}
	s.config.Store(&Config{ChannelIDs: []int{}, TokenIDs: []int{}})
	return s
}

func (s *Service) Apply(c Config) {
	// Published slices must not be modified by the caller or an API consumer.
	c.ChannelIDs = normalizeIDs(c.ChannelIDs)
	c.TokenIDs = normalizeIDs(c.TokenIDs)
	s.config.Store(&c)
}

func (s *Service) Config() Config {
	c := *s.config.Load()
	c.ChannelIDs = slices.Clone(c.ChannelIDs)
	c.TokenIDs = slices.Clone(c.TokenIDs)
	return c
}

func (s *Service) ConfigJSON() string {
	value, _ := json.Marshal(s.config.Load())
	return string(value)
}

func (s *Service) SetRecorder(r *Recorder) {
	if r != nil {
		r.service.Store(s)
	}
	s.recorder.Store(r)
}

type Status struct {
	Enabled     bool   `json:"enabled"`
	Config      Config `json:"config"`
	ConfigError string `json:"config_error"`
	Stats
}

func (s *Service) Status() Status {
	c := s.Config()
	status := Status{Enabled: c.Enabled(), Config: c}
	if message := s.configError.Load(); message != nil {
		status.ConfigError = *message
	}
	if r := s.recorder.Load(); r != nil {
		status.Stats = r.Status()
	}
	return status
}

func (s *Service) SetConfigError(err error) {
	message := ""
	if err != nil {
		message = clip(err.Error())
	}
	s.configError.Store(&message)
}
