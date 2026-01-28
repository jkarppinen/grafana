package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/api/response"
	"github.com/grafana/grafana/pkg/services/publicdashboards"
	. "github.com/grafana/grafana/pkg/services/publicdashboards/models"
	"github.com/grafana/grafana/pkg/web"
)

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
			accessToken: "abc123",
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
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(dto PublicDashboardQueryDTO) bool {
					// Verify that variables are passed correctly to the service
					return dto.Variables != nil &&
						dto.Variables["server"] == "localhost" &&
						dto.Variables["interval"] == "5m" &&
						dto.Variables["env"] == "production"
				}), int64(1), "abc123").Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: []*backend.DataFrameGroup{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:        "should handle multi-value variables",
			accessToken: "abc123",
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
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(dto PublicDashboardQueryDTO) bool {
					servers, serverOk := dto.Variables["servers"].([]interface{})
					metrics, metricsOk := dto.Variables["metrics"].([]interface{})

					return serverOk && metricsOk &&
						len(servers) == 3 && servers[0] == "server1" &&
						len(metrics) == 3 && metrics[0] == "cpu"
				}), int64(2), "abc123").Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: []*backend.DataFrameGroup{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:        "should handle empty variables",
			accessToken: "abc123",
			panelId:     "3",
			queryDTO: PublicDashboardQueryDTO{
				IntervalMs:      1000,
				MaxDataPoints:   100,
				QueryCachingTTL: 60,
				TimeRange: TimeRangeDTO{
					From:     "now-1h",
					To:       "now",
					Timezone: "UTC",
				},
				Variables: map[string]interface{}{},
			},
			mockSetup: func(service *publicdashboards.FakePublicDashboardService) {
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(dto PublicDashboardQueryDTO) bool {
					return dto.Variables != nil && len(dto.Variables) == 0
				}), int64(3), "abc123").Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: []*backend.DataFrameGroup{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:        "should handle nil variables",
			accessToken: "abc123",
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
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(dto PublicDashboardQueryDTO) bool {
					return dto.Variables == nil
				}), int64(4), "abc123").Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: []*backend.DataFrameGroup{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:        "should handle complex variable types",
			accessToken: "abc123",
			panelId:     "5",
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
					"string_var": "test",
					"number_var": 42,
					"bool_var":   true,
					"null_var":   nil,
					"array_var":  []interface{}{"a", "b", "c"},
					"nested_var": map[string]interface{}{
						"key": "value",
					},
				},
			},
			mockSetup: func(service *publicdashboards.FakePublicDashboardService) {
				service.On("GetQueryDataResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(dto PublicDashboardQueryDTO) bool {
					vars := dto.Variables
					return vars["string_var"] == "test" &&
						vars["number_var"] == 42 &&
						vars["bool_var"] == true &&
						vars["null_var"] == nil &&
						len(vars["array_var"].([]interface{})) == 3
				}), int64(5), "abc123").Return(&backend.QueryDataResponse{
					Responses: map[string]backend.DataResponse{
						"A": {
							Frames: []*backend.DataFrameGroup{},
						},
					},
				}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			name:                 "should return error for invalid panel id",
			accessToken:          "abc123",
			panelId:              "invalid",
			queryDTO:             PublicDashboardQueryDTO{},
			mockSetup:            func(service *publicdashboards.FakePublicDashboardService) {},
			expectedStatusCode:   400,
			expectedErrorMessage: "QueryPublicDashboard: error parsing panelId",
		},
		{
			name:                 "should return error for invalid access token",
			accessToken:          "",
			panelId:              "1",
			queryDTO:             PublicDashboardQueryDTO{},
			mockSetup:            func(service *publicdashboards.FakePublicDashboardService) {},
			expectedStatusCode:   400,
			expectedErrorMessage: "QueryPublicDashboard: invalid access token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := &publicdashboards.FakePublicDashboardService{}
			tc.mockSetup(service)

			api := &Api{
				PublicDashboardService: service,
			}

			// Create request body
			bodyBytes, err := json.Marshal(tc.queryDTO)
			require.NoError(t, err)
			body := strings.NewReader(string(bodyBytes))

			// Create HTTP request
			req, err := http.NewRequest("POST", "/api/public/dashboards/"+tc.accessToken+"/panels/"+tc.panelId+"/query", body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// Create request context with URL parameters
			ctx := context.Background()
			ctx = web.AddParamsToContext(ctx, map[string]string{
				":accessToken": tc.accessToken,
				":panelId":     tc.panelId,
			})
			req = req.WithContext(ctx)

			// Create response recorder
			recorder := &response.NormalResponse{
				Status:     200,
				Body:       nil,
				Header:     make(http.Header),
				ErrMessage: "",
				ErrStatus:  0,
			}

			// Create request context
			reqCtx := &web.ReqContext{
				Req: req,
			}

			// Call the API
			resp := api.QueryPublicDashboard(reqCtx)

			// Assert response
			if tc.expectedStatusCode == 200 {
				assert.IsType(t, &response.NormalResponse{}, resp)
				service.AssertExpectations(t)
			} else {
				// For error cases, check that an error response is returned
				errorResp, ok := resp.(*response.ErrResponse)
				require.True(t, ok, "Expected error response")
				assert.Contains(t, errorResp.Message, tc.expectedErrorMessage)
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
				assert.Equal(t, tc.variables, unmarshaled.Variables)
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

	// Verify variables are preserved correctly
	assert.Equal(t, "production", unmarshaled.Variables["environment"])
	assert.Equal(t, 95.5, unmarshaled.Variables["threshold"])
	assert.Equal(t, true, unmarshaled.Variables["enabled"])
	assert.Nil(t, unmarshaled.Variables["optional"])

	// Verify array variable
	services := unmarshaled.Variables["services"].([]interface{})
	assert.Len(t, services, 3)
	assert.Equal(t, "api", services[0])
	assert.Equal(t, "web", services[1])
	assert.Equal(t, "worker", services[2])
}
