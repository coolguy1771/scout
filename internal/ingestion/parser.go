package ingestion

import (
	"encoding/json"
	"fmt"
	"io"

	"go.uber.org/zap"
)

// GeoJSONFeature represents a GeoJSON feature
type GeoJSONFeature struct {
	Type       string                 `json:"type"`
	Geometry   map[string]interface{} `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

// GeoJSONFeatureCollection represents a GeoJSON FeatureCollection
type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

// ParsedFeature represents a parsed feature ready for database insertion
type ParsedFeature struct {
	Geometry   string                 // GeoJSON string
	Properties map[string]interface{} // Feature properties
	Type       string                 // Geometry type: Point, LineString, Polygon, etc.
}

// Parser handles parsing of geospatial file formats
type Parser struct {
	logger *zap.Logger
}

// NewParser creates a new parser
func NewParser(logger *zap.Logger) *Parser {
	return &Parser{logger: logger}
}

// ParseGeoJSON parses a GeoJSON file and returns parsed features
func (p *Parser) ParseGeoJSON(reader io.Reader) ([]ParsedFeature, error) {
	var geojsonData map[string]interface{}

	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&geojsonData); err != nil {
		return nil, fmt.Errorf("failed to decode GeoJSON: %w", err)
	}

	// Handle FeatureCollection
	if geojsonData["type"] == "FeatureCollection" {
		var collection GeoJSONFeatureCollection
		if err := json.Unmarshal(mustMarshal(geojsonData), &collection); err != nil {
			return nil, fmt.Errorf("failed to parse FeatureCollection: %w", err)
		}

		features := make([]ParsedFeature, 0, len(collection.Features))
		for _, feature := range collection.Features {
			parsed, err := p.parseFeature(feature)
			if err != nil {
				p.logger.Warn("Failed to parse feature", zap.Error(err))
				continue
			}
			features = append(features, parsed)
		}

		return features, nil
	}

	// Handle single Feature
	if geojsonData["type"] == "Feature" {
		var feature GeoJSONFeature
		if err := json.Unmarshal(mustMarshal(geojsonData), &feature); err != nil {
			return nil, fmt.Errorf("failed to parse Feature: %w", err)
		}

		parsed, err := p.parseFeature(feature)
		if err != nil {
			return nil, err
		}

		return []ParsedFeature{parsed}, nil
	}

	// Handle Geometry object directly
	if geometry, ok := geojsonData["type"].(string); ok {
		geometryJSON := mustMarshal(geojsonData)
		properties := make(map[string]interface{})
		if props, ok := geojsonData["properties"].(map[string]interface{}); ok {
			properties = props
		}

		return []ParsedFeature{
			{
				Geometry:   string(geometryJSON),
				Properties: properties,
				Type:       geometry,
			},
		}, nil
	}

	return nil, fmt.Errorf("unsupported GeoJSON type: %v", geojsonData["type"])
}

// parseFeature parses a single GeoJSON feature
func (p *Parser) parseFeature(feature GeoJSONFeature) (ParsedFeature, error) {
	geometryJSON, err := json.Marshal(feature.Geometry)
	if err != nil {
		return ParsedFeature{}, fmt.Errorf("failed to marshal geometry: %w", err)
	}

	geometryType := ""
	if gt, ok := feature.Geometry["type"].(string); ok {
		geometryType = gt
	}

	return ParsedFeature{
		Geometry:   string(geometryJSON),
		Properties: feature.Properties,
		Type:       geometryType,
	}, nil
}

// DetermineLayerType determines the layer type from parsed features
func (p *Parser) DetermineLayerType(features []ParsedFeature) string {
	if len(features) == 0 {
		return "mixed"
	}

	firstType := features[0].Type
	for _, feature := range features {
		if feature.Type != firstType {
			return "mixed"
		}
	}

	// Map GeoJSON types to layer types
	switch firstType {
	case "Point", "MultiPoint":
		return "point"
	case "LineString", "MultiLineString":
		return "line"
	case "Polygon", "MultiPolygon":
		return "polygon"
	default:
		return "mixed"
	}
}

// mustMarshal marshals an interface to JSON, panicking on error
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return data
}


