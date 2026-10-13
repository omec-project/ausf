// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package callback

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const applicationJSON = "application/json"

// Route is the information for every URI.
type Route struct {
	// Name is the name of this Route.
	Name string
	// Method is the string for the HTTP method (e.g., GET, POST, etc.)
	Method string
	// Pattern is the pattern of the URI.
	Pattern string
	// HandlerFunc is the handler function of this route.
	HandlerFunc gin.HandlerFunc
}

// Routes is the list of the generated Route.
type Routes []Route

func AddService(engine *gin.Engine) *gin.RouterGroup {
	group := engine.Group("/nausf-callback/v1")
	for _, route := range getRoutes() {
		switch route.Method {
		case http.MethodGet:
			group.GET(route.Pattern, route.HandlerFunc)
		case http.MethodPost:
			group.POST(route.Pattern, route.HandlerFunc)
		case http.MethodPut:
			group.PUT(route.Pattern, route.HandlerFunc)
		case http.MethodPatch:
			group.PATCH(route.Pattern, route.HandlerFunc)
		case http.MethodDelete:
			group.DELETE(route.Pattern, route.HandlerFunc)
		}
	}
	return group
}

func getRoutes() []Route {
	return []Route{
		{
			Name:        "NfStatusNotify",
			Method:      http.MethodPost,
			Pattern:     "/nf-status-notify",
			HandlerFunc: HTTPNfSubscriptionStatusNotify,
		},
	}
}
