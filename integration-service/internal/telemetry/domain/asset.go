// Package domain defines the telemetry bounded context domain model and rules.
package domain

import "fmt"

type AssetID string

type AssetType string

const (
	DefaultAssetID   AssetID   = "871689260010377213"
	SolarPanelType   AssetType = "solar_panel"
	defaultAssetName           = "solar_panel_871689260010377213"
)

type Asset struct {
	id              AssetID
	assetType       AssetType
	name            string
	registerMapping RegisterMapping
}

func NewAsset(id AssetID, assetType AssetType, name string, registerMapping RegisterMapping) (Asset, error) {
	switch {
	case id == "":
		return Asset{}, fmt.Errorf("asset id must not be empty")
	case assetType == "":
		return Asset{}, fmt.Errorf("asset type must not be empty")
	case name == "":
		return Asset{}, fmt.Errorf("asset name must not be empty")
	}

	return Asset{
		id:              id,
		assetType:       assetType,
		name:            name,
		registerMapping: registerMapping,
	}, nil
}

func NewDefaultAsset() Asset {
	asset, err := NewAsset(
		DefaultAssetID,
		SolarPanelType,
		defaultAssetName,
		NewDefaultRegisterMapping(),
	)
	if err != nil {
		panic("domain: default asset configuration is invalid")
	}

	return asset
}

func (id AssetID) String() string {
	return string(id)
}

func (a Asset) ID() AssetID {
	return a.id
}

func (a Asset) Type() AssetType {
	return a.assetType
}

func (a Asset) Name() string {
	return a.name
}

func (a Asset) RegisterMapping() RegisterMapping {
	return a.registerMapping
}
