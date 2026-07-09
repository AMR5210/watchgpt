package middleware

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

const (
	cognitoAccessTokenUse = "access"
	jwksCacheTTL          = time.Hour
)

type CognitoConfig struct {
	Region      string
	UserPoolID  string
	AppClientID string
}

func (c CognitoConfig) Enabled() bool {
	return c.Region != "" && c.UserPoolID != "" && c.AppClientID != ""
}

func (c CognitoConfig) PartiallyConfigured() bool {
	return c.UserPoolID != "" || c.AppClientID != ""
}

func (c CognitoConfig) Issuer() string {
	return fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", c.Region, c.UserPoolID)
}

type cognitoVerifier struct {
	issuer      string
	appClientID string
	jwksURL     string
	httpClient  *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type cognitoClaims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	TokenUse  string `json:"token_use"`
	ClientID  string `json:"client_id"`
	Username  string `json:"username"`
	Scope     string `json:"scope"`
	Expires   int64  `json:"exp"`
	NotBefore int64  `json:"nbf,omitempty"`
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func CognitoAuth(config CognitoConfig) func(http.Handler) http.Handler {
	verifier := newCognitoVerifier(config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := verifier.verify(r.Context(), token)
			if err != nil {
				requestctx.Logger(r.Context()).Warn("jwt verification failed", "error", err)
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			user := requestctx.User{
				ID:       claims.Subject,
				Username: claims.Username,
				Scopes:   strings.Fields(claims.Scope),
			}
			next.ServeHTTP(w, r.WithContext(requestctx.WithUser(r.Context(), user)))
		})
	}
}

func newCognitoVerifier(config CognitoConfig) *cognitoVerifier {
	issuer := config.Issuer()
	return &cognitoVerifier{
		issuer:      issuer,
		appClientID: config.AppClientID,
		jwksURL:     issuer + "/.well-known/jwks.json",
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		keys:        make(map[string]*rsa.PublicKey),
	}
}

func (v *cognitoVerifier) verify(ctx context.Context, token string) (cognitoClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return cognitoClaims{}, errors.New("jwt must have three parts")
	}

	var header jwtHeader
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return cognitoClaims{}, fmt.Errorf("decode header: %w", err)
	}
	if header.Alg != "RS256" {
		return cognitoClaims{}, fmt.Errorf("unexpected jwt alg: %s", header.Alg)
	}
	if header.Kid == "" {
		return cognitoClaims{}, errors.New("jwt kid is missing")
	}

	var claims cognitoClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return cognitoClaims{}, fmt.Errorf("decode claims: %w", err)
	}

	key, err := v.keyForID(ctx, header.Kid)
	if err != nil {
		return cognitoClaims{}, err
	}

	signed := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return cognitoClaims{}, fmt.Errorf("decode signature: %w", err)
	}
	digest := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return cognitoClaims{}, fmt.Errorf("verify signature: %w", err)
	}

	if err := v.validateClaims(claims, time.Now()); err != nil {
		return cognitoClaims{}, err
	}

	return claims, nil
}

func (v *cognitoVerifier) validateClaims(claims cognitoClaims, now time.Time) error {
	if claims.Issuer != v.issuer {
		return fmt.Errorf("unexpected issuer: %s", claims.Issuer)
	}
	if claims.TokenUse != cognitoAccessTokenUse {
		return fmt.Errorf("unexpected token_use: %s", claims.TokenUse)
	}
	if claims.ClientID != v.appClientID {
		return errors.New("unexpected client_id")
	}
	if claims.Subject == "" {
		return errors.New("subject is missing")
	}

	nowUnix := now.Unix()
	if claims.Expires <= nowUnix {
		return errors.New("token expired")
	}
	if claims.NotBefore != 0 && claims.NotBefore > nowUnix {
		return errors.New("token not active yet")
	}

	return nil
}

func (v *cognitoVerifier) keyForID(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	cacheValid := time.Now().Before(v.expiresAt)
	v.mu.RUnlock()
	if ok && cacheValid {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("jwks key not found for kid %s", kid)
	}
	return key, nil
}

func (v *cognitoVerifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks returned %d", resp.StatusCode)
	}

	var body jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, jwk := range body.Keys {
		key, err := jwk.rsaPublicKey()
		if err != nil {
			return fmt.Errorf("parse jwk %s: %w", jwk.Kid, err)
		}
		keys[jwk.Kid] = key
	}
	if len(keys) == 0 {
		return errors.New("jwks has no usable keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.expiresAt = time.Now().Add(jwksCacheTTL)
	v.mu.Unlock()
	return nil
}

func (j jwk) rsaPublicKey() (*rsa.PublicKey, error) {
	if j.Kid == "" || j.Kty != "RSA" || j.N == "" || j.E == "" {
		return nil, errors.New("invalid rsa jwk")
	}
	if j.Alg != "" && j.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported alg: %s", j.Alg)
	}
	if j.Use != "" && j.Use != "sig" {
		return nil, fmt.Errorf("unsupported key use: %s", j.Use)
	}

	modulusBytes, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}

	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 + int(b)
	}
	if exponent == 0 {
		return nil, errors.New("invalid exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulusBytes),
		E: exponent,
	}, nil
}

func decodeJWTPart(part string, out any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, out)
}
