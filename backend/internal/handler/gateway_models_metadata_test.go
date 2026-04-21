//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildModelResponse_EnrichesOptionalMetadata(t *testing.T) {
	t.Parallel()

	h := &GatewayHandler{}
	resp := h.buildModelResponse("gpt-5.4", "gpt-5.4", "2024-01-01T00:00:00Z")

	require.Equal(t, "gpt-5.4", resp.ID)
	require.Equal(t, "model", resp.Type)
	require.Equal(t, "gpt-5.4", resp.DisplayName)
	require.Equal(t, "2024-01-01T00:00:00Z", resp.CreatedAt)
	require.NotNil(t, resp.ContextLength)
	require.Equal(t, 1050000, *resp.ContextLength)
	require.NotNil(t, resp.MaxOutputTokens)
	require.Equal(t, 128000, *resp.MaxOutputTokens)
	require.NotNil(t, resp.Pricing)
	require.NotNil(t, resp.Pricing.InputCostPerToken)
	require.InDelta(t, 2.5e-06, *resp.Pricing.InputCostPerToken, 1e-12)
	require.NotNil(t, resp.Pricing.OutputCostPerToken)
	require.InDelta(t, 1.5e-05, *resp.Pricing.OutputCostPerToken, 1e-12)
}

func TestBuildModelResponse_OmitsUnknownOptionalMetadata(t *testing.T) {
	t.Parallel()

	h := &GatewayHandler{}
	resp := h.buildModelResponse("custom-unknown-model", "custom-unknown-model", "2024-01-01T00:00:00Z")

	require.Nil(t, resp.ContextLength)
	require.Nil(t, resp.MaxOutputTokens)
	require.Nil(t, resp.Pricing)
}

func TestModels_WhitelistResponseIncludesMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	h := &GatewayHandler{}
	h.writeModelsResponse(c, []string{"gpt-5.4"})

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"id":"gpt-5.4"`)
	require.Contains(t, body, `"context_length":1050000`)
	require.Contains(t, body, `"max_output_tokens":128000`)
	require.Contains(t, body, `"pricing"`)
}
