package geocode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeocode(t *testing.T) {
	tests := []struct {
		name string
		in   Address
		want Point
	}{
		{"postal code wins", Address{City: "Somewhere", PostalCode: "77002", Country: "US"}, Point{29.7604, -95.3698}},
		{"city case insensitive", Address{City: "  NEW YORK ", Country: "us"}, Point{40.7128, -74.0060}},
		{"city with accent", Address{City: "Bogotá", Country: "CO"}, Point{4.7110, -74.0721}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TableGeocoder{}.Geocode(context.Background(), tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGeocode_FallbackIsDeterministicAndInsideCountryBox(t *testing.T) {
	a := Address{City: "Springfield", PostalCode: "62701", Country: "US"}
	first, err := TableGeocoder{}.Geocode(context.Background(), a)
	require.NoError(t, err)
	second, err := TableGeocoder{}.Geocode(context.Background(), a)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	us := boxes["US"]
	assert.True(t, first.Lat >= us.minLat && first.Lat <= us.maxLat)
	assert.True(t, first.Lng >= us.minLng && first.Lng <= us.maxLng)
}

func TestGeocode_Errors(t *testing.T) {
	_, err := TableGeocoder{}.Geocode(context.Background(), Address{Line1: "1 Main St", Country: "US"})
	var gErr *GeocodingError
	require.ErrorAs(t, err, &gErr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = TableGeocoder{}.Geocode(ctx, Address{City: "Houston", Country: "US"})
	require.ErrorAs(t, err, &gErr)
}
