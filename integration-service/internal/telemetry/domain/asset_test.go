package domain

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type AssetTestSuite struct {
	suite.Suite
	validMapping RegisterMapping
}

func TestAssetTestSuite(t *testing.T) {
	suite.Run(t, new(AssetTestSuite))
}

func (s *AssetTestSuite) SetupTest() {
	s.validMapping = NewDefaultRegisterMapping()
}

func (s *AssetTestSuite) TestNewAsset() {
	tests := []struct {
		name        string
		id          AssetID
		assetType   AssetType
		assetName   string
		wantErrText string
	}{
		{
			name:        "empty asset id rejected",
			id:          "",
			assetType:   SolarPanelType,
			assetName:   "panel-a",
			wantErrText: "asset id must not be empty",
		},
		{
			name:        "empty asset type rejected",
			id:          "asset-1",
			assetType:   "",
			assetName:   "panel-a",
			wantErrText: "asset type must not be empty",
		},
		{
			name:        "empty asset name rejected",
			id:          "asset-1",
			assetType:   SolarPanelType,
			assetName:   "",
			wantErrText: "asset name must not be empty",
		},
		{
			name:      "valid asset creation accepted",
			id:        "asset-1",
			assetType: SolarPanelType,
			assetName: "panel-a",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			asset, err := NewAsset(tt.id, tt.assetType, tt.assetName, s.validMapping)
			if tt.wantErrText == "" {
				s.Require().NoError(err)
				s.Equal(tt.id, asset.ID())
				s.Equal(tt.assetType, asset.Type())
				s.Equal(tt.assetName, asset.Name())
				s.Equal(s.validMapping, asset.RegisterMapping())
				return
			}

			s.Require().Error(err)
			s.Equal(tt.wantErrText, err.Error())
		})
	}
}

func (s *AssetTestSuite) TestNewDefaultAsset() {
	asset := NewDefaultAsset()
	expectedMapping := NewDefaultRegisterMapping()

	s.Equal(DefaultAssetID, asset.ID())
	s.Equal(SolarPanelType, asset.Type())
	s.NotEmpty(asset.Name())
	s.Equal(expectedMapping, asset.RegisterMapping())
}
