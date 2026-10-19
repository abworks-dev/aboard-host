package protocol

import (
	"fmt"
	"net/url"
	"strings"
)

// Route is a value projection of generated operation metadata, not another route
// or authorization authority. Path IDs are still validated before use.
type Route struct {
	ID, Action string
	Parameters map[string]string
}

func MatchRoute(method, path string) (Route, bool) {
	for _, op := range operations {
		if strings.ToUpper(op.Method) != method {
			continue
		}
		pattern, parts := strings.Split(op.Path, "/"), strings.Split(path, "/")
		if len(pattern) != len(parts) {
			continue
		}
		values := make(map[string]string)
		matched := true
		for i, p := range pattern {
			if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
				if parts[i] == "" {
					matched = false
					break
				}
				values[p[1:len(p)-1]] = parts[i]
			} else if p != parts[i] {
				matched = false
				break
			}
		}
		if matched {
			return Route{ID: op.ID, Action: op.Action, Parameters: values}, true
		}
	}
	return Route{}, false
}

// ValidateRouteParameters rejects unknown/duplicate query keys and validates all
// required parameters against the same generated schemas used by the DTOs.
func ValidateRouteParameters(route Route, query url.Values) error {
	for _, op := range operations {
		if op.ID != route.ID {
			continue
		}
		seen := make(map[string]bool)
		for _, p := range op.Parameters {
			if p.In == "path" {
				v, ok := route.Parameters[p.Name]
				if !ok || ValidateParameter(op.ID, "path", p.Name, v) != nil {
					return fmt.Errorf("invalid path parameter")
				}
				continue
			}
			if p.In != "query" {
				continue
			}
			seen[p.Name] = true
			vs, ok := query[p.Name]
			if !ok {
				if p.Required {
					return fmt.Errorf("missing query parameter")
				}
				continue
			}
			if len(vs) != 1 || ValidateParameter(op.ID, "query", p.Name, vs[0]) != nil {
				return fmt.Errorf("invalid query parameter")
			}
		}
		for k := range query {
			if !seen[k] {
				return fmt.Errorf("unknown query parameter")
			}
		}
		return nil
	}
	return fmt.Errorf("unknown operation")
}
