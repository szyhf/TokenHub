package server

func (s *Server) pricedEmbeddingRoutes(call CallContext, routes []RouteSelection) []RouteSelection {
	return s.pricedRetrievalRoutes(call, routes, "embedding", providerRouteProtocolEmbeddings)
}

// Inventory can change independently of published routes. Check the execution
// snapshot on every request, including routes supplied by scoped provider hooks.
func (s *Server) pricedRetrievalRoutes(call CallContext, routes []RouteSelection, modality, protocol string) []RouteSelection {
	result := make([]RouteSelection, 0, len(routes))
	models := s.store.ListProviderModels()
	for _, route := range routes {
		if !s.providerRetrievalSupport(route.Provider, modality) && !s.hasGatewayProviderCallHookForRoute(call, route, protocol) {
			continue
		}
		search := modality == "rerank" && providerRerankProtocol(route.Provider) == "cohere"
		if !retrievalPriceConfigured(call.Model, search, false) {
			continue
		}
		// The explicit mock adapter has no external procurement contract.
		if route.Provider.Type == ProviderMock {
			result = append(result, route)
			continue
		}
		for _, upstream := range models {
			if upstream.ProviderID != route.Provider.ID || upstream.UpstreamModel != route.ProviderModel {
				continue
			}
			if canonicalProviderModality(upstream.Modality) != modality || !retrievalTextInputSupported(upstream) {
				break
			}
			if retrievalPriceConfigured(providerModelCostModel(upstream), search, true) {
				result = append(result, route)
			}
			break
		}
	}
	return result
}
