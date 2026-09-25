package server

import (
	"sort"
	"strings"
)

// Catalog assembly helpers shared by provider create and catalog preview
// flows. Pure functions over store and catalog data; no HTTP concerns.

func (s *Server) providerCatalogEntryWithSelectedStandardModels(entry ProviderCatalogEntry, selectedModels []string, category string) ProviderCatalogEntry {
	modelsByName := map[string]Model{}
	for _, model := range s.store.ListModels() {
		modelsByName[normalizeModelLookupName(model.Name)] = model
	}
	defaultCategory := providerCatalogEntrySelectedModelCategory(entry, category)
	models := make([]ProviderCatalogModel, 0, len(selectedModels))
	for _, modelID := range selectedModels {
		model, ok := modelsByName[normalizeModelLookupName(modelID)]
		if !ok {
			continue
		}
		modelCategory := standardModelCategory(firstNonEmpty(defaultCategory, model.Category, inferModelCategory(model.Name, model.Name)))
		models = append(models, ProviderCatalogModel{
			ID:                        model.Name,
			Name:                      model.Name,
			DisplayName:               firstNonEmpty(model.Metadata["display_name"], model.Name),
			CanonicalName:             model.Name,
			Category:                  modelCategory,
			Family:                    model.Family,
			Type:                      model.Modality,
			ContextWindow:             model.ContextWindow,
			InputPriceUSDPer1M:        model.InputPriceUSDPer1M,
			CacheReadPriceUSDPer1M:    model.CacheReadPriceUSDPer1M,
			CacheWritePriceUSDPer1M:   model.CacheWritePriceUSDPer1M,
			CacheWrite5mPriceUSDPer1M: model.CacheWrite5mPriceUSDPer1M,
			CacheWrite1hPriceUSDPer1M: model.CacheWrite1hPriceUSDPer1M,
			OutputPriceUSDPer1M:       model.OutputPriceUSDPer1M,
			InputModalities:           append([]string(nil), model.InputModalities...),
			OutputModalities:          append([]string(nil), model.OutputModalities...),
			Capabilities:              append([]string(nil), model.Capabilities...),
			SupportedParameters:       append([]string(nil), model.SupportedParameters...),
			Metadata:                  cloneStringMap(model.Metadata),
		})
	}
	catalog := entry
	if len(models) > 0 {
		catalog.Categories, catalog.CategoryCounts = catalogCategorySummary(models)
	}
	catalog.Models = models
	catalog.ModelsCount = len(models)
	return catalog
}

func providerCatalogEntrySelectedModelCategory(entry ProviderCatalogEntry, requestedCategory string) string {
	if category := standardModelCategory(requestedCategory); category != "" && category != "all" {
		return category
	}
	for _, category := range entry.Categories {
		if category = standardModelCategory(category); category != "" && category != "all" {
			return category
		}
	}
	return ""
}

func (s *Server) customProviderCatalogFromStandardModels(category string) ProviderCatalogEntry {
	models := []ProviderCatalogModel{}
	normalizedCategory := standardModelCategory(category)
	for _, model := range s.store.ListModels() {
		modelCategory := standardModelCategory(firstNonEmpty(model.Category, inferModelCategory(model.Name, model.Name)))
		if normalizedCategory != "" && normalizedCategory != "all" && modelCategory != normalizedCategory {
			continue
		}
		models = append(models, ProviderCatalogModel{
			ID:                     model.Name,
			Name:                   model.Name,
			DisplayName:            model.Name,
			CanonicalName:          model.Name,
			Category:               modelCategory,
			Family:                 model.Family,
			Type:                   model.Modality,
			ContextWindow:          model.ContextWindow,
			InputPriceUSDPer1M:     model.InputPriceUSDPer1M,
			CacheReadPriceUSDPer1M: model.CacheReadPriceUSDPer1M,
			OutputPriceUSDPer1M:    model.OutputPriceUSDPer1M,
			InputModalities:        append([]string(nil), model.InputModalities...),
			OutputModalities:       append([]string(nil), model.OutputModalities...),
			Capabilities:           append([]string(nil), model.Capabilities...),
			SupportedParameters:    append([]string(nil), model.SupportedParameters...),
			Metadata:               map[string]string{"source": "tokenhub-standard-catalog"},
		})
	}
	categories, categoryCounts := catalogCategorySummary(models)
	if len(models) == 0 {
		entry := s.providerCatalog.customProviderCatalogEntry()
		entry.Categories = []string{firstNonEmpty(normalizedCategory, "custom")}
		entry.CategoryCounts = map[string]int{firstNonEmpty(normalizedCategory, "custom"): 0}
		entry.Models = nil
		entry.ModelsCount = 0
		return entry
	}
	entry := s.providerCatalog.customProviderCatalogEntry()
	entry.Categories = categories
	entry.CategoryCounts = categoryCounts
	entry.Models = models
	entry.ModelsCount = len(models)
	return entry
}

func customProviderCatalogFromModelsWithType(input []ProviderCatalogModel, category string, providerType string) ProviderCatalogEntry {
	normalizedCategory := strings.TrimSpace(category)
	if normalizedCategory != "" {
		normalizedCategory = standardModelCategory(normalizedCategory)
	}
	models := make([]ProviderCatalogModel, 0, len(input))
	seen := map[string]bool{}
	for _, model := range input {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		model.Name = firstNonEmpty(strings.TrimSpace(model.Name), model.ID)
		model.DisplayName = firstNonEmpty(strings.TrimSpace(model.DisplayName), model.Name)
		model.CanonicalName = firstNonEmpty(strings.TrimSpace(model.CanonicalName), canonicalModelName(model.ID, model.DisplayName))
		model.Category = standardModelCategory(firstNonEmpty(model.Category, inferModelCategory(model.ID, model.DisplayName)))
		if normalizedCategory != "" && normalizedCategory != "all" && model.Category != normalizedCategory {
			continue
		}
		model.Family = firstNonEmpty(model.Family, inferModelFamily(model.ID))
		model.Type = firstNonEmpty(model.Type, normalizeModelModality(model.ID))
		if model.Metadata == nil {
			model.Metadata = map[string]string{}
		}
		if model.Metadata["source"] == "" {
			model.Metadata["source"] = "custom-upstream"
		}
		models = append(models, model)
	}
	sort.SliceStable(models, func(i, j int) bool {
		return strings.ToLower(models[i].ID) < strings.ToLower(models[j].ID)
	})
	entry := customProviderCatalogEntryWithType(providerType)
	entry.Source = "custom-upstream"
	entry.Models = models
	entry.ModelsCount = len(models)
	entry.Categories, entry.CategoryCounts = catalogCategorySummary(models)
	return entry
}
