package middleware

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"
)

func TestCognitoVerifierAcceptsValidToken(t *testing.T) {
	key := mustGenerateRSAKey(t)
	kid := "test-kid"
	issuer := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	clientID := "test-client"

	verifier := testVerifier(issuer, clientID, kid, &key.PublicKey)
	token := signTestJWT(t, key, kid, map[string]any{
		"iss":       issuer,
		"sub":       "user-123",
		"token_use": "access",
		"client_id": clientID,
		"username":  "akshay",
		"scope":     "openid email",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	claims, err := verifier.verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Fatalf("subject = %q, want user-123", claims.Subject)
	}
}

func TestCognitoVerifierRejectsWrongClientID(t *testing.T) {
	key := mustGenerateRSAKey(t)
	kid := "test-kid"
	issuer := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"

	verifier := testVerifier(issuer, "expected-client", kid, &key.PublicKey)
	token := signTestJWT(t, key, kid, map[string]any{
		"iss":       issuer,
		"sub":       "user-123",
		"token_use": "access",
		"client_id": "wrong-client",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	if _, err := verifier.verify(context.Background(), token); err == nil {
		t.Fatal("verify succeeded, want error")
	}
}

func TestCognitoVerifierRejectsExpiredToken(t *testing.T) {
	key := mustGenerateRSAKey(t)
	kid := "test-kid"
	issuer := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	clientID := "test-client"

	verifier := testVerifier(issuer, clientID, kid, &key.PublicKey)
	token := signTestJWT(t, key, kid, map[string]any{
		"iss":       issuer,
		"sub":       "user-123",
		"token_use": "access",
		"client_id": clientID,
		"exp":       time.Now().Add(-time.Minute).Unix(),
	})

	if _, err := verifier.verify(context.Background(), token); err == nil {
		t.Fatal("verify succeeded, want error")
	}
}

func TestJWKParsesRSAPublicKey(t *testing.T) {
	key := mustGenerateRSAKey(t)
	parsed, err := testJWK("test-kid", &key.PublicKey).rsaPublicKey()
	if err != nil {
		t.Fatalf("parse jwk: %v", err)
	}
	if parsed.N.Cmp(key.PublicKey.N) != 0 || parsed.E != key.PublicKey.E {
		t.Fatal("parsed key does not match original key")
	}
}

func testVerifier(issuer, clientID, kid string, key *rsa.PublicKey) *cognitoVerifier {
	return &cognitoVerifier{
		issuer:      issuer,
		appClientID: clientID,
		keys:        map[string]*rsa.PublicKey{kid: key},
		expiresAt:   time.Now().Add(time.Hour),
	}
}

func signTestJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	header := map[string]any{
		"alg": "RS256",
		"kid": kid,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}

	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func testJWK(kid string, key *rsa.PublicKey) jwk {
	return jwk{
		Kid: kid,
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func mustGenerateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
