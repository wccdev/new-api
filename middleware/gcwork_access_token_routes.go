package middleware

import "fmt"

// DeclareAccessTokenScopeRoute records the access token scope a downstream
// (GC Work) route requires. It runs while routes are registered, so the rule
// exists only when the route does, and panics on conflicting declarations.
func DeclareAccessTokenScopeRoute(method, fullPath, scope string) {
	key := method + " " + fullPath
	rule := accessTokenScopeRule(scope)
	if _, ok := accessTokenRouteRules[key]; ok {
		panic(fmt.Sprintf("access token route %s is declared twice", key))
	}
	permissionRouteRulesMu.Lock()
	defer permissionRouteRulesMu.Unlock()
	if existing, ok := permissionRouteRules[key]; ok && existing != rule {
		panic(fmt.Sprintf("access token route %s is declared with %s and %s", key, existing.scope, rule.scope))
	}
	permissionRouteRules[key] = rule
}
