package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"done-hub/model"

	"github.com/spf13/viper"
)

const providerValidationTTL = 10 * time.Minute

type providerValidationClaims struct {
	Digest    string `json:"digest"`
	ExpiresAt int64  `json:"expires_at"`
}

type providerValidationInput struct {
	Type              int    `json:"type"`
	ProtocolProfileID string `json:"protocol_profile_id"`
	Key               string `json:"key"`
	BaseURL           string `json:"base_url"`
	Proxy             string `json:"proxy"`
	Other             string `json:"other"`
	TestModel         string `json:"test_model"`
}

func issueProviderValidationToken(channel *model.Channel) (string, error) {
	key, err := providerValidationSigningKey()
	if err != nil {
		return "", err
	}
	claims := providerValidationClaims{
		Digest:    providerValidationDigest(channel),
		ExpiresAt: time.Now().Add(providerValidationTTL).Unix(),
	}
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(rawClaims)
	signature := providerValidationSignature(key, payload)
	return payload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func verifyProviderValidationToken(channel *model.Channel) error {
	parts := strings.Split(channel.ValidationToken, ".")
	if len(parts) != 2 {
		return errors.New("连接探测验证已缺失，请重新执行连接探测")
	}
	key, err := providerValidationSigningKey()
	if err != nil {
		return err
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(providedSignature, providerValidationSignature(key, parts[0])) {
		return errors.New("连接探测验证无效，请重新执行连接探测")
	}
	rawClaims, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("连接探测验证无效，请重新执行连接探测")
	}
	var claims providerValidationClaims
	if err := json.Unmarshal(rawClaims, &claims); err != nil {
		return errors.New("连接探测验证无效，请重新执行连接探测")
	}
	if time.Now().Unix() > claims.ExpiresAt {
		return errors.New("连接探测验证已过期，请重新执行连接探测")
	}
	if !hmac.Equal([]byte(claims.Digest), []byte(providerValidationDigest(channel))) {
		return errors.New("连接配置在探测后发生变化，请重新执行连接探测")
	}
	return nil
}

func providerValidationDigest(channel *model.Channel) string {
	input := providerValidationInput{
		Type:              channel.Type,
		ProtocolProfileID: strings.TrimSpace(channel.ProtocolProfileID),
		Key:               strings.TrimSpace(channel.Key),
		BaseURL:           strings.TrimSpace(channel.GetBaseURL()),
		Proxy:             strings.TrimSpace(channel.GetProxy()),
		Other:             strings.TrimSpace(channel.Other),
		TestModel:         strings.TrimSpace(channel.TestModel),
	}
	raw, _ := json.Marshal(input)
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func providerValidationSigningKey() ([]byte, error) {
	value := viper.GetString("gateway_secret_key")
	if value == "" {
		value = viper.GetString("user_token_secret")
	}
	if value == "" {
		return nil, errors.New("连接探测签名密钥未配置")
	}
	return []byte(value), nil
}

func providerValidationSignature(key []byte, payload string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
