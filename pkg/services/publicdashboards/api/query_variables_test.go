package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/services/publicdashboards"
	. "github.com/grafana/grafana/pkg/services/publicdashboards/models"
	"github.com/grafana/grafana/pkg/services/publicdashboards/service"
)

var testValidAccessToken, _ = service.GenerateAccessToken()

func TestQueryPublicDashboardWithVariables(t *testing.T) {
	testCases := []struct {
		name                 string
		accessToken          string
		panelId              string
		queryDTO             PublicDashboardQueryDTO
		mockSetup            func(*publicdashboards.FakePublicDashboardService)
		expectedStatusCode   int
		expectedErrorMessage string
	}{
		{
			name:        "should successfully query with variables",
			accessToken: testValidAccessToken,
			panelId:     "1",
			queryDTO: PublicDashboardQueryDTO{
				IntervalMs:      1000,
				MaxDataPoints:   100,
				QueryCachingTTL: 60,
				TimeRange: TimeRangeDTO{
					From:     "now-1h",
					To:       "now",
					Timezone: "UTC",
				},
				Variables: map[string]interface{}{
					"server":   "localhost",
					"interval": "5m",
					"env":      "production",
				},
			},
			mockSetup: func(service *publicdashboards.FakePublicDashboardService) {
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.Anything, int64(1), testValidAccessToken).Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: data.Frames{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:        "should handle multi-value variables",
			accessToken: testValidAccessToken,
			panelId:     "2",
			queryDTO: PublicDashboardQueryDTO{
				IntervalMs:      1000,
				MaxDataPoints:   100,
				QueryCachingTTL: 60,
				TimeRange: TimeRangeDTO{
					From:     "now-1h",
					To:       "now",
					Timezone: "UTC",
				},
				Variables: map[string]interface{}{
					"servers": []interface{}{"server1", "server2", "server3"},
					"metrics": []interface{}{"cpu", "memory", "disk"},
				},
			},
			mockSetup: func(service *publicdashboards.FakePublicDashboardService) {
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.Anything, int64(2), testValidAccessToken).Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: data.Frames{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:        "should handle nil variables",
			accessToken: testValidAccessToken,
			panelId:     "4",
			queryDTO: PublicDashboardQueryDTO{
				IntervalMs:      1000,
				MaxDataPoints:   100,
				QueryCachingTTL: 60,
				TimeRange: TimeRangeDTO{
					From:     "now-1h",
					To:       "now",
					Timezone: "UTC",
				},
				Variables: nil,
			},
			mockSetup: func(service *publicdashboards.FakePublicDashboardService) {
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.Anything, int64(4), testValidAccessToken).Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: data.Frames{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := publicdashboards.NewFakePublicDashboardService(t)
			tc.mockSetup(service)

			testServer := setupTestServer(t, nil, service, anonymousUser)

			// Create request body
			bodyBytes, err := json.Marshal(tc.queryDTO)
			require.NoError(t, err)
			body := strings.NewReader(string(bodyBytes))

			// Call API endpoint
			path := fmt.Sprintf("/api/public/dashboards/%s/panels/%s/query", tc.accessToken, tc.panelId)
			resp := callAPI(testServer, http.MethodPost, path, body, t)

			// Assert response
			assert.Equal(t, tc.expectedStatusCode, resp.Code)

			if tc.expectedStatusCode == 200 {
				service.AssertExpectations(t)
			} else if tc.expectedErrorMessage != "" {
				// For error cases, check the response body contains the error message
				assert.Contains(t, resp.Body.String(), tc.expectedErrorMessage)
			}
		})
	}
}

func TestQueryPublicDashboardVariableValidation(t *testing.T) {
	testCases := []struct {
		name      string
		variables map[string]interface{}
		valid     bool
	}{
		{
			name: "valid simple variables",
			variables: map[string]interface{}{
				"server": "localhost",
				"port":   8080,
			},
			valid: true,
		},
		{
			name: "valid array variables",
			variables: map[string]interface{}{
				"servers": []interface{}{"server1", "server2"},
				"ports":   []interface{}{8080, 9090},
			},
			valid: true,
		},
		{
			name: "handles nil values",
			variables: map[string]interface{}{
				"optional_var": nil,
				"required_var": "value",
			},
			valid: true,
		},
		{
			name: "handles boolean values",
			variables: map[string]interface{}{
				"enabled": true,
				"debug":   false,
			},
			valid: true,
		},
		{
			name: "handles mixed types",
			variables: map[string]interface{}{
				"string":  "test",
				"number":  42,
				"boolean": true,
				"array":   []interface{}{"a", "b"},
				"null":    nil,
			},
			valid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			queryDTO := PublicDashboardQueryDTO{
				Variables: tc.variables,
			}

			// Test JSON marshaling/unmarshaling
			data, err := json.Marshal(queryDTO)
			require.NoError(t, err)

			var unmarshaled PublicDashboardQueryDTO
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)

			if tc.valid {
				// Verify the variables map has the same keys
				assert.Equal(t, len(tc.variables), len(unmarshaled.Variables))
				for key := range tc.variables {
					assert.Contains(t, unmarshaled.Variables, key)
				}
			}
		})
	}
}

func TestPublicDashboardQueryDTOSerialization(t *testing.T) {
	queryDTO := PublicDashboardQueryDTO{
		IntervalMs:      1000,
		MaxDataPoints:   500,
		QueryCachingTTL: 300,
		TimeRange: TimeRangeDTO{
			From:     "now-6h",
			To:       "now",
			Timezone: "UTC",
		},
		Variables: map[string]interface{}{
			"environment": "production",
			"services":    []interface{}{"api", "web", "worker"},
			"threshold":   95.5,
			"enabled":     true,
			"optional":    nil,
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(queryDTO)
	require.NoError(t, err)

	// Test JSON unmarshaling
	var unmarshaled PublicDashboardQueryDTO
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify all fields are preserved
	assert.Equal(t, queryDTO.IntervalMs, unmarshaled.IntervalMs)
	assert.Equal(t, queryDTO.MaxDataPoints, unmarshaled.MaxDataPoints)
	assert.Equal(t, queryDTO.QueryCachingTTL, unmarshaled.QueryCachingTTL)
	assert.Equal(t, queryDTO.TimeRange, unmarshaled.TimeRange)

	// Verify variables map has same keys
	assert.Equal(t, len(queryDTO.Variables), len(unmarshaled.Variables))
	for key := range queryDTO.Variables {
		assert.Contains(t, unmarshaled.Variables, key)
	}
}
