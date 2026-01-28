package api

import (
	"net/http"

	"github.com/grafana/grafana/pkg/api/response"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	. "github.com/grafana/grafana/pkg/services/publicdashboards/models"
	"github.com/grafana/grafana/pkg/services/publicdashboards/validation"
	"github.com/grafana/grafana/pkg/web"
)

// swagger:route POST /public/dashboards/{accessToken}/variables/{variableName}/query dashboards dashboard_public queryPublicDashboardVariable
//
//	Get variable options for a public dashboard
//
// Responses:
// 200: queryPublicDashboardVariableResponse
// 400: badRequestPublicError
// 401: unauthorisedPublicError
// 404: notFoundPublicError
// 403: forbiddenPublicError
// 500: internalServerPublicError
func (api *Api) QueryPublicDashboardVariable(c *contextmodel.ReqContext) response.Response {
	accessToken := web.Params(c.Req)[":accessToken"]
	if !validation.IsValidAccessToken(accessToken) {
		return response.Err(ErrInvalidAccessToken.Errorf("QueryPublicDashboardVariable: invalid access token"))
	}

	variableName := web.Params(c.Req)[":variableName"]
	if variableName == "" {
		return response.Err(ErrBadRequest.Errorf("QueryPublicDashboardVariable: variable name is required"))
	}

	reqDTO := PublicDashboardVariableQueryDTO{}
	if err := web.Bind(c.Req, &reqDTO); err != nil {
		return response.Err(ErrBadRequest.Errorf("QueryPublicDashboardVariable: error parsing request: %v", err))
	}

	options, err := api.PublicDashboardService.GetVariableQueryResponse(c.Req.Context(), accessToken, variableName, reqDTO)
	if err != nil {
		return response.Err(err)
	}

	return response.JSON(http.StatusOK, options)
}

// swagger:response queryPublicDashboardVariableResponse
type QueryPublicDashboardVariableResponse struct {
	// in: body
	Body []MetricFindValue `json:"body"`
}

// swagger:parameters queryPublicDashboardVariable
type QueryPublicDashboardVariableParams struct {
	// in: path
	AccessToken string `json:"accessToken"`
	// in: path
	VariableName string `json:"variableName"`
	// in: body
	Body PublicDashboardVariableQueryDTO
}
