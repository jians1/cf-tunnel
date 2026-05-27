package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJWTValidatorRequiresAccessToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	validator := NewJWTValidator("team", "", []string{"aud"})

	result, err := validator.Handle(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, result.StatusCode)
	assert.True(t, result.ShouldFilterRequest)
	assert.Equal(t, "no access token in request", result.Reason)
}

func TestJWTValidatorUnsupportedInMinimalBuild(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set(headerKeyAccessJWTAssertion, "jwt")
	validator := NewJWTValidator("team", "", []string{"aud"})

	result, err := validator.Handle(context.Background(), req)

	assert.Nil(t, result)
	assert.EqualError(t, err, "Cloudflare Access JWT validation is not supported in this minimal build")
}
