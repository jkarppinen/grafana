package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana/pkg/api/dtos"
	"github.com/grafana/grafana/pkg/apimachinery/identity"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/services/dashboards"
	"github.com/grafana/grafana/pkg/services/publicdashboards/models"
)

// GetVariableQueryResponse returns the options for a template variable in a public dashboard
func (pd *PublicDashboardServiceImpl) GetVariableQueryResponse(ctx context.Context, accessToken string, variableName string, reqDTO models.PublicDashboardVariableQueryDTO) ([]models.MetricFindValue, error) {
	ctx, span := tracer.Start(ctx, "publicdashboards.GetVariableQueryResponse")
	defer span.End()

	// Find the public dashboard and dashboard by access token
	publicDashboard, dashboard, err := pd.FindEnabledPublicDashboardAndDashboardByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	// Find the variable definition in the dashboard
	variable, err := pd.findVariableInDashboard(dashboard, variableName)
	if err != nil {
		return nil, err
	}

	// Get variable options based on variable type
	options, err := pd.getVariableOptions(ctx, dashboard, publicDashboard, variable, reqDTO)
	if err != nil {
		return nil, err
	}

	// Apply search filter if provided
	if reqDTO.SearchFilter != "" {
		options = filterVariableOptions(options, reqDTO.SearchFilter)
	}

	return options, nil
}

// variableDefinition represents a template variable from the dashboard JSON
type variableDefinition struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Query      interface{}            `json:"query"`
	Datasource map[string]interface{} `json:"datasource"`
	Options    []variableOption       `json:"options"`
	Current    variableCurrent        `json:"current"`
	Multi      bool                   `json:"multi"`
	Refresh    int                    `json:"refresh"`
	Regex      string                 `json:"regex"`
	Sort       int                    `json:"sort"`
}

type variableOption struct {
	Text     interface{} `json:"text"`
	Value    interface{} `json:"value"`
	Selected bool        `json:"selected"`
}

type variableCurrent struct {
	Text  interface{} `json:"text"`
	Value interface{} `json:"value"`
}

// findVariableInDashboard finds a variable definition in the dashboard's templating.list
func (pd *PublicDashboardServiceImpl) findVariableInDashboard(dashboard *dashboards.Dashboard, variableName string) (*variableDefinition, error) {
	templating := dashboard.Data.Get("templating")
	if templating.Interface() == nil {
		return nil, models.ErrVariableNotFound.Errorf("findVariableInDashboard: no templating section found in dashboard")
	}

	list := templating.Get("list")
	if list.Interface() == nil {
		return nil, models.ErrVariableNotFound.Errorf("findVariableInDashboard: no templating.list found in dashboard")
	}

	for _, varInterface := range list.MustArray() {
		varJSON := simplejson.NewFromAny(varInterface)
		name := varJSON.Get("name").MustString()
		if name == variableName {
			// Convert to variableDefinition struct
			varBytes, err := varJSON.Encode()
			if err != nil {
				return nil, models.ErrInternalServerError.Errorf("findVariableInDashboard: failed to encode variable: %w", err)
			}

			var variable variableDefinition
			if err := json.Unmarshal(varBytes, &variable); err != nil {
				return nil, models.ErrInternalServerError.Errorf("findVariableInDashboard: failed to unmarshal variable: %w", err)
			}

			return &variable, nil
		}
	}

	return nil, models.ErrVariableNotFound.Errorf("findVariableInDashboard: variable '%s' not found", variableName)
}

// getVariableOptions returns options based on the variable type
func (pd *PublicDashboardServiceImpl) getVariableOptions(ctx context.Context, dashboard *dashboards.Dashboard, publicDashboard *models.PublicDashboard, variable *variableDefinition, reqDTO models.PublicDashboardVariableQueryDTO) ([]models.MetricFindValue, error) {
	var options []models.MetricFindValue
	var err error

	switch variable.Type {
	case "query":
		options, err = pd.getQueryVariableOptions(ctx, dashboard, publicDashboard, variable, reqDTO)
	case "custom":
		options, err = pd.getCustomVariableOptions(variable)
	case "constant":
		options, err = pd.getConstantVariableOptions(variable)
	case "interval":
		options, err = pd.getIntervalVariableOptions(variable)
	default:
		// For unsupported types, return existing options if available
		options, err = pd.getStaticVariableOptions(variable)
	}

	if err != nil {
		return []models.MetricFindValue{}, err
	}

	// If no options found, try to return the current value as a fallback
	if len(options) == 0 {
		options = pd.getCurrentValueAsOption(variable)
	}

	// Ensure we never return nil (which marshals to null in JSON)
	if options == nil {
		options = []models.MetricFindValue{}
	}

	return options, nil
}

// getCurrentValueAsOption returns the variable's current value as a single option
func (pd *PublicDashboardServiceImpl) getCurrentValueAsOption(variable *variableDefinition) []models.MetricFindValue {
	if variable.Current.Value == nil {
		return []models.MetricFindValue{}
	}

	var options []models.MetricFindValue

	switch v := variable.Current.Value.(type) {
	case string:
		if v != "" {
			text := v
			if t, ok := variable.Current.Text.(string); ok && t != "" {
				text = t
			}
			options = append(options, models.MetricFindValue{
				Text:  text,
				Value: v,
			})
		}
	case []interface{}:
		texts, _ := variable.Current.Text.([]interface{})
		for i, val := range v {
			if valStr, ok := val.(string); ok && valStr != "" {
				text := valStr
				if texts != nil && i < len(texts) {
					if t, ok := texts[i].(string); ok {
						text = t
					}
				}
				options = append(options, models.MetricFindValue{
					Text:  text,
					Value: valStr,
				})
			}
		}
	}

	return options
}

// getQueryVariableOptions executes a datasource query to get variable options
func (pd *PublicDashboardServiceImpl) getQueryVariableOptions(ctx context.Context, dashboard *dashboards.Dashboard, publicDashboard *models.PublicDashboard, variable *variableDefinition, reqDTO models.PublicDashboardVariableQueryDTO) ([]models.MetricFindValue, error) {
	// Get the datasource UID from the variable definition
	dsUID := ""
	dsType := ""
	if variable.Datasource != nil {
		if uid, ok := variable.Datasource["uid"].(string); ok {
			dsUID = uid
		}
		if t, ok := variable.Datasource["type"].(string); ok {
			dsType = t
		}
	}

	pd.log.Info("getQueryVariableOptions: processing variable", "variable", variable.Name, "dsUID", dsUID, "dsType", dsType, "queryRaw", variable.Query)

	if dsUID == "" {
		// If no datasource specified, return empty options
		pd.log.Warn("getQueryVariableOptions: variable has no datasource", "variable", variable.Name)
		return []models.MetricFindValue{}, nil
	}

	// Get the query from the variable - preserve the full query object for datasources that need it
	var queryObj map[string]interface{}
	queryStr := ""

	switch q := variable.Query.(type) {
	case string:
		queryStr = q
		queryObj = map[string]interface{}{
			"query": queryStr,
		}
	case map[string]interface{}:
		queryObj = q
		if query, ok := q["query"].(string); ok {
			queryStr = query
		}
	}

	pd.log.Info("getQueryVariableOptions: extracted query", "variable", variable.Name, "queryStr", queryStr)

	// Apply variable interpolation to the query if other variables are provided
	if reqDTO.Variables != nil && queryStr != "" {
		queryStr = pd.interpolateVariables(queryStr, reqDTO.Variables)
		queryObj["query"] = queryStr
	}

	// Build the query object with all necessary fields
	queryData := map[string]interface{}{
		"refId":      "A",
		"datasource": variable.Datasource,
	}

	// Copy all fields from the original query object
	for k, v := range queryObj {
		queryData[k] = v
	}

	// Also set common query fields that different datasources might use
	if queryStr != "" {
		queryData["query"] = queryStr
		queryData["expr"] = queryStr      // Prometheus uses expr
		queryData["rawQuery"] = true      // Some datasources need this
		queryData["rawSql"] = queryStr    // SQL datasources
	}

	// Build a metric request for the variable query
	metricReq := dtos.MetricRequest{
		From:    "now-1h",
		To:      "now",
		Queries: []*simplejson.Json{simplejson.NewFromAny(queryData)},
	}

	pd.log.Info("getQueryVariableOptions: executing query", "variable", variable.Name, "queryData", queryData)

	// Use service identity to execute the query
	svcCtx, svcIdent := identity.WithServiceIdentity(ctx, dashboard.OrgID)

	// Execute the query
	res, err := pd.QueryDataService.QueryData(svcCtx, svcIdent, false, metricReq)
	if err != nil {
		pd.log.Error("getQueryVariableOptions: query failed", "error", err, "variable", variable.Name)
		return []models.MetricFindValue{}, nil
	}

	pd.log.Info("getQueryVariableOptions: query succeeded", "variable", variable.Name)

	// Extract options from the response
	return pd.extractOptionsFromQueryResponse(res)
}

// extractOptionsFromQueryResponse extracts MetricFindValue options from a query response
func (pd *PublicDashboardServiceImpl) extractOptionsFromQueryResponse(res *backend.QueryDataResponse) ([]models.MetricFindValue, error) {
	options := make([]models.MetricFindValue, 0)

	pd.log.Info("extractOptionsFromQueryResponse: processing response", "numResponses", len(res.Responses))

	for refId, response := range res.Responses {
		if response.Error != nil {
			pd.log.Warn("extractOptionsFromQueryResponse: response has error", "refId", refId, "error", response.Error)
			continue
		}

		pd.log.Info("extractOptionsFromQueryResponse: processing frames", "refId", refId, "numFrames", len(response.Frames))

		for _, frame := range response.Frames {
			if frame == nil {
				continue
			}

			// Try to extract text/value from the frame fields
			var textField, valueField *data.Field
			for i, field := range frame.Fields {
				if field == nil {
					continue
				}
				name := strings.ToLower(field.Name)
				if name == "text" || name == "__text" || name == "name" || name == "label" {
					textField = frame.Fields[i]
				}
				if name == "value" || name == "__value" || name == "id" {
					valueField = frame.Fields[i]
				}
			}

			// If we couldn't find specific text/value fields, use the first field for both
			if len(frame.Fields) > 0 && frame.Fields[0] != nil {
				if textField == nil {
					textField = frame.Fields[0]
				}
				if valueField == nil {
					valueField = textField
				}
			}

			if textField == nil {
				continue
			}

			// Extract values from the fields
			for i := 0; i < textField.Len(); i++ {
				text := fieldValueToString(textField.At(i))
				value := text
				if valueField != nil && i < valueField.Len() {
					value = fieldValueToString(valueField.At(i))
				}
				if text != "" {
					options = append(options, models.MetricFindValue{
						Text:  text,
						Value: value,
					})
				}
			}
		}
	}

	return options, nil
}

// fieldValueToString converts a data.Field value to string, handling pointers
func fieldValueToString(v interface{}) string {
	if v == nil {
		return ""
	}

	// Handle pointer types (nullable fields return pointers)
	switch val := v.(type) {
	case *string:
		if val == nil {
			return ""
		}
		return *val
	case string:
		return val
	case *float64:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", *val)
	case float64:
		return fmt.Sprintf("%v", val)
	case *float32:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", *val)
	case float32:
		return fmt.Sprintf("%v", val)
	case *int64:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%d", *val)
	case int64:
		return fmt.Sprintf("%d", val)
	case *int32:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%d", *val)
	case int32:
		return fmt.Sprintf("%d", val)
	case *int:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%d", *val)
	case int:
		return fmt.Sprintf("%d", val)
	case *bool:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", *val)
	case bool:
		return fmt.Sprintf("%v", val)
	default:
		// For any other type, use reflection-safe string conversion
		return fmt.Sprintf("%v", v)
	}
}

// getCustomVariableOptions parses the query field as comma-separated values
func (pd *PublicDashboardServiceImpl) getCustomVariableOptions(variable *variableDefinition) ([]models.MetricFindValue, error) {
	var options []models.MetricFindValue

	queryStr, ok := variable.Query.(string)
	if !ok {
		return options, nil
	}

	// Custom variables have comma-separated values in the query field
	values := strings.Split(queryStr, ",")
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			// Check if value contains a colon for text:value format
			parts := strings.SplitN(v, ":", 2)
			if len(parts) == 2 {
				options = append(options, models.MetricFindValue{
					Text:  strings.TrimSpace(parts[0]),
					Value: strings.TrimSpace(parts[1]),
				})
			} else {
				options = append(options, models.MetricFindValue{
					Text:  v,
					Value: v,
				})
			}
		}
	}

	return options, nil
}

// getConstantVariableOptions returns the constant value as a single option
func (pd *PublicDashboardServiceImpl) getConstantVariableOptions(variable *variableDefinition) ([]models.MetricFindValue, error) {
	var options []models.MetricFindValue

	// Get value from current
	value := ""
	switch v := variable.Current.Value.(type) {
	case string:
		value = v
	case []interface{}:
		if len(v) > 0 {
			value = fmt.Sprintf("%v", v[0])
		}
	}

	if value != "" {
		text := value
		if t, ok := variable.Current.Text.(string); ok && t != "" {
			text = t
		}
		options = append(options, models.MetricFindValue{
			Text:  text,
			Value: value,
		})
	}

	return options, nil
}

// getIntervalVariableOptions returns pre-defined interval options
func (pd *PublicDashboardServiceImpl) getIntervalVariableOptions(variable *variableDefinition) ([]models.MetricFindValue, error) {
	// Return the options from the variable definition if available
	if len(variable.Options) > 0 {
		return pd.getStaticVariableOptions(variable)
	}

	// Default interval options
	intervals := []string{"1m", "5m", "10m", "30m", "1h", "6h", "12h", "1d", "7d", "14d", "30d"}
	var options []models.MetricFindValue

	for _, interval := range intervals {
		options = append(options, models.MetricFindValue{
			Text:  interval,
			Value: interval,
		})
	}

	return options, nil
}

// getStaticVariableOptions returns existing options from the variable definition
func (pd *PublicDashboardServiceImpl) getStaticVariableOptions(variable *variableDefinition) ([]models.MetricFindValue, error) {
	var options []models.MetricFindValue

	for _, opt := range variable.Options {
		text := ""
		value := ""

		switch t := opt.Text.(type) {
		case string:
			text = t
		case []interface{}:
			if len(t) > 0 {
				text = fmt.Sprintf("%v", t[0])
			}
		}

		switch v := opt.Value.(type) {
		case string:
			value = v
		case []interface{}:
			if len(v) > 0 {
				value = fmt.Sprintf("%v", v[0])
			}
		}

		if text != "" || value != "" {
			if text == "" {
				text = value
			}
			if value == "" {
				value = text
			}
			options = append(options, models.MetricFindValue{
				Text:  text,
				Value: value,
			})
		}
	}

	return options, nil
}

// filterVariableOptions filters options by a search filter
func filterVariableOptions(options []models.MetricFindValue, filter string) []models.MetricFindValue {
	filter = strings.ToLower(filter)
	var filtered []models.MetricFindValue

	for _, opt := range options {
		if strings.Contains(strings.ToLower(opt.Text), filter) ||
			strings.Contains(strings.ToLower(opt.Value), filter) {
			filtered = append(filtered, opt)
		}
	}

	return filtered
}
