package main

import (
	"fmt"
	"net/http"
	"strings"
)

type Route struct {
	Actions []Action
}

// Action contains information about how a specified action should be handled.
type Action struct {
	ActionName  string
	Callback    func(*Response) string
	ServiceType string
}

func NewRoute() Route {
	return Route{}
}

// RoutingGroup defines a group of actions for a given service type.
type RoutingGroup struct {
	Route       *Route
	ServiceType string
}

// HandleGroup returns a routing group type for the given service type.
func (r *Route) HandleGroup(serviceType string) RoutingGroup {
	return RoutingGroup{
		Route:       r,
		ServiceType: serviceType,
	}
}

func (r *RoutingGroup) Handle(action string, function func(*Response) string) {
	r.Route.Actions = append(r.Route.Actions, Action{
		ActionName:  action,
		Callback:    function,
		ServiceType: r.ServiceType,
	})
}

func (r *Route) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		paths := strings.Split(req.URL.Path, "/")
		if req.Method == "POST" {
			req.ParseForm()
		}

		// Ensure we can route to this action before processing.
		// Search all registered actions and find a matching action.
		var action Action
		for _, routeAction := range r.Actions {
			if routeAction.ActionName == paths[2] && routeAction.ServiceType == paths[1] {
				action = routeAction
			}
		}

		// Action is only properly populated if we found it previously.
		if action.ActionName == "" && action.ServiceType == "" {
			// printError(w, "Unknown action was passed.", http.StatusBadRequest)
			return
		}

		resp := &Response{
			request: req,
			writer:  &w,
		}

		payload := action.Callback(resp)
		w.Write([]byte(payload))
	})
}

func ConvertToCGI(response CGIResponse) string {
	var work string
	if response.code != 0 {
		work += fmt.Sprintf("cd=%d\n", response.code)
	}

	if response.message != "" {
		work += fmt.Sprintf("msg=%s\n", response.message)
	}

	for _, kv := range response.other {
		work += fmt.Sprintf("%s=%s\n", kv.key, kv.value)
	}

	return work
}
