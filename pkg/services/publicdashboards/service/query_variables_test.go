package service

import (
	"testing"

	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/services/dashboards"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyTemplateVariables(t *testing.T) {
	service := &PublicDashboardServiceImpl{
		log: log.NewNopLogger(),
	}

	testCases := []struct {
		name           string
		dashboardJSON  string
		variables      map[string]interface{}
		expectedResult string
	}{
		{
			name: "should replace simple variable in panel query",
			dashboardJSON: `{
				"panels": [
					{
						"id": 1,
						"targets": [
							{
								"expr": "up{instance=\"${server}\"}",
								"refId": "A"
							}
						]
					}
				]
			}`,
			variables: map[string]interface{}{
				"server": "localhost:9090",
			},
			expectedResult: `up{instance="localhost:9090"}`,
		},
		{
			name: "should replace multiple variable formats",
			dashboardJSON: `{
				"panels": [
					{
						"id": 1,
						"targets": [
							{
								"expr": "rate($metric[${interval}])",
								"refId": "A"
							}
						]
					}
				]
			}`,
			variables: map[string]interface{}{
				"metric":   "cpu_usage",
				"interval": "5m",
			},
			expectedResult: `rate(cpu_usage[5m])`,
		},
		{
			name: "should handle multi-value variables",
			dashboardJSON: `{
				"panels": [
					{
						"id": 1,
						"targets": [
							{
								"expr": "up{instance=~\"${servers}\"}",
								"refId": "A"
							}
						]
					}
				]
			}`,
			variables: map[string]interface{}{
				"servers": []interface{}{"server1", "server2", "server3"},
			},
			expectedResult: `up{instance=~"server1,server2,server3"}`,
		},
		{
			name: "should not replace undefined variables",
			dashboardJSON: `{
				"panels": [
					{
						"id": 1,
						"targets": [
							{
								"expr": "up{instance=\"${undefined_var}\"}",
								"refId": "A"
							}
						]
					}
				]
			}`,
			variables: map[string]interface{}{
				"defined_var": "value",
			},
			expectedResult: `up{instance="${undefined_var}"}`,
		},
		{
			name: "should handle null and empty variables",
			dashboardJSON: `{
				"panels": [
					{
						"id": 1,
						"targets": [
							{
								"expr": "up{instance=\"${null_var}\"}",
								"refId": "A"
							}
						]
					}
				]
			}`,
			variables: map[string]interface{}{
				"null_var": nil,
			},
			expectedResult: `up{instance="${null_var}"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the dashboard JSON
			dashboardData, err := simplejson.NewJson([]byte(tc.dashboardJSON))
			require.NoError(t, err)

			dashboard := &dashboards.Dashboard{
				UID:  "test-uid",
				Data: dashboardData,
			}

			// Apply template variables
			result := service.applyTemplateVariables(dashboard, tc.variables)

			// Extract the expr from the first panel's first target
			panels := result.Data.Get("panels").MustArray()
			require.Len(t, panels, 1)

			panel := panels[0].(map[string]interface{})
			targets := panel["targets"].([]interface{})
			require.Len(t, targets, 1)

			target := targets[0].(map[string]interface{})
			actualExpr := target["expr"].(string)

			assert.Equal(t, tc.expectedResult, actualExpr)
		})
	}
}

func TestInterpolateVariables(t *testing.T) {
	service := &PublicDashboardServiceImpl{
		log: log.NewNopLogger(),
	}

	testCases := []struct {
		name      string
		text      string
		variables map[string]interface{}
		expected  string
	}{
		{
			name: "should replace variables in ${} format",
			text: "SELECT * FROM table WHERE col = ${myVar}",
			variables: map[string]interface{}{
				"myVar": "test-value",
			},
			expected: "SELECT * FROM table WHERE col = test-value",
		},
		{
			name: "should replace variables in $ format",
			text: "SELECT * FROM table WHERE col = $myVar",
			variables: map[string]interface{}{
				"myVar": "test-value",
			},
			expected: "SELECT * FROM table WHERE col = test-value",
		},
		{
			name: "should handle multiple variables",
			text: "SELECT $field FROM $table WHERE id = ${id}",
			variables: map[string]interface{}{
				"field": "name",
				"table": "users",
				"id":    "123",
			},
			expected: "SELECT name FROM users WHERE id = 123",
		},
		{
			name: "should handle multi-value variables",
			text: "SELECT * FROM table WHERE col IN (${values})",
			variables: map[string]interface{}{
				"values": []interface{}{"a", "b", "c"},
			},
			expected: "SELECT * FROM table WHERE col IN (a,b,c)",
		},
		{
			name: "should handle number variables",
			text: "SELECT * FROM table LIMIT ${limit}",
			variables: map[string]interface{}{
				"limit": 100,
			},
			expected: "SELECT * FROM table LIMIT 100",
		},
		{
			name: "should not replace partial matches",
			text: "This has $variable not $var",
			variables: map[string]interface{}{
				"var": "test",
			},
			expected: "This has $variable not test",
		},
		{
			name:      "should handle empty variables map",
			text:      "SELECT * FROM table WHERE col = ${myVar}",
			variables: map[string]interface{}{},
			expected:  "SELECT * FROM table WHERE col = ${myVar}",
		},
		{
			name: "should handle special regex characters in variable names",
			text: "SELECT * FROM table WHERE col = ${my.var-name}",
			variables: map[string]interface{}{
				"my.var-name": "test-value",
			},
			expected: "SELECT * FROM table WHERE col = test-value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := service.interpolateVariables(tc.text, tc.variables)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestApplyTemplateVariablesWithComplexDashboard(t *testing.T) {
	service := &PublicDashboardServiceImpl{
		log: log.NewNopLogger(),
	}

	dashboardJSON := `{
		"title": "Dashboard with ${env} environment",
		"panels": [
			{
				"id": 1,
				"title": "Panel for ${service}",
				"targets": [
					{
						"expr": "rate(${metric}[${interval}])",
						"legendFormat": "${service} - {{instance}}",
						"refId": "A"
					},
					{
						"expr": "up{service=~\"${services}\"}",
						"refId": "B"
					}
				]
			}
		],
		"templating": {
			"list": [
				{
					"name": "env",
					"current": {"value": "production"}
				},
				{
					"name": "service",
					"current": {"value": "api"}
				}
			]
		}
	}`

	variables := map[string]interface{}{
		"env":      "production",
		"service":  "api-service",
		"metric":   "http_requests_total",
		"interval": "5m",
		"services": []interface{}{"api", "web", "worker"},
	}

	dashboardData, err := simplejson.NewJson([]byte(dashboardJSON))
	require.NoError(t, err)

	dashboard := &dashboards.Dashboard{
		UID:  "test-uid",
		Data: dashboardData,
	}

	result := service.applyTemplateVariables(dashboard, variables)

	// Check title interpolation
	title := result.Data.Get("title").MustString()
	assert.Equal(t, "Dashboard with production environment", title)

	// Check panel title interpolation
	panels := result.Data.Get("panels").MustArray()
	panel := panels[0].(map[string]interface{})
	panelTitle := panel["title"].(string)
	assert.Equal(t, "Panel for api-service", panelTitle)

	// Check target interpolation
	targets := panel["targets"].([]interface{})
	target1 := targets[0].(map[string]interface{})
	assert.Equal(t, "rate(http_requests_total[5m])", target1["expr"])
	assert.Equal(t, "api-service - {{instance}}", target1["legendFormat"])

	target2 := targets[1].(map[string]interface{})
	assert.Equal(t, "up{service=~\"api,web,worker\"}", target2["expr"])
}

func TestApplyTemplateVariablesInvalidJSON(t *testing.T) {
	service := &PublicDashboardServiceImpl{
		log: log.NewNopLogger(), // Add logger to avoid nil pointer
	}

	// Test with a dashboard that has valid JSON but will be handled gracefully
	dashboard := &dashboards.Dashboard{
		UID:  "test-uid",
		Data: simplejson.New(),
	}
	// Add some normal data that can be processed
	dashboard.Data.Set("title", "Test Dashboard")
	dashboard.Data.Set("panels", []interface{}{
		map[string]interface{}{
			"id":    1,
			"title": "Panel with ${variable}",
			"targets": []interface{}{
				map[string]interface{}{
					"expr": "up{instance=\"${server}\"}",
				},
			},
		},
	})

	variables := map[string]interface{}{
		"variable": "test-var",
		"server":   "localhost",
	}

	// This should successfully process variables and return a new dashboard
	result := service.applyTemplateVariables(dashboard, variables)

	// Should return a different dashboard instance (not the same reference)
	assert.NotEqual(t, dashboard, result)
	// Should have interpolated the variables
	panels := result.Data.Get("panels").MustArray()
	panel := panels[0].(map[string]interface{})
	assert.Equal(t, "Panel with test-var", panel["title"])
}
