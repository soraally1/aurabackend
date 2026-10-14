// Package geoutils provides spatial calculations, Haversine distance,
// and Geohash encoding/bounding-box utilities for proximity detection.
package geoutils

import (
	"math"
	"strings"
)

const (
	// EarthRadiusKm is the approximate mean radius of the Earth in kilometers.
	EarthRadiusKm = 6371.0
	base32Chars   = "0123456789bcdefghjkmnpqrstuvwxyz"
)

// Coordinates represents a latitude and longitude pair.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// BoundingBox represents a rectangular geographic bounding box.
type BoundingBox struct {
	MinLat float64 `json:"min_lat"`
	MaxLat float64 `json:"max_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLon float64 `json:"max_lon"`
}

// HaversineDistance computes the great-circle distance between two points in kilometers.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := degreesToRadians(lat2 - lat1)
	dLon := degreesToRadians(lon2 - lon1)

	rLat1 := degreesToRadians(lat1)
	rLat2 := degreesToRadians(lat2)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rLat1)*math.Cos(rLat2)*math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return EarthRadiusKm * c
}

func degreesToRadians(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// CalculateBoundingBox returns the bounding box given a center point and radius in kilometers.
func CalculateBoundingBox(lat, lon, radiusKm float64) BoundingBox {
	latDelta := (radiusKm / EarthRadiusKm) * (180.0 / math.Pi)
	lonDelta := (radiusKm / (EarthRadiusKm * math.Cos(degreesToRadians(lat)))) * (180.0 / math.Pi)

	return BoundingBox{
		MinLat: lat - latDelta,
		MaxLat: lat + latDelta,
		MinLon: lon - lonDelta,
		MaxLon: lon + lonDelta,
	}
}

// EncodeGeohash generates a standard geohash string for the given coordinates and precision.
func EncodeGeohash(latitude, longitude float64, precision int) string {
	if precision <= 0 {
		precision = 6
	}

	latRange := [2]float64{-90.0, 90.0}
	lonRange := [2]float64{-180.0, 180.0}

	var sb strings.Builder
	isEven := true
	bit := 0
	ch := 0

	for sb.Len() < precision {
		if isEven {
			mid := (lonRange[0] + lonRange[1]) / 2
			if longitude >= mid {
				ch |= 1 << (4 - bit)
				lonRange[0] = mid
			} else {
				lonRange[1] = mid
			}
		} else {
			mid := (latRange[0] + latRange[1]) / 2
			if latitude >= mid {
				ch |= 1 << (4 - bit)
				latRange[0] = mid
			} else {
				latRange[1] = mid
			}
		}

		isEven = !isEven
		if bit < 4 {
			bit++
		} else {
			sb.WriteByte(base32Chars[ch])
			bit = 0
			ch = 0
		}
	}

	return sb.String()
}

// GeohashQueryRange represents lower and upper bounds for prefix-based indexing queries.
type GeohashQueryRange struct {
	Lower string
	Upper string
}

// GetGeohashQueryRange returns a range query [Lower, Upper] for a given geohash prefix.
func GetGeohashQueryRange(geohash string) GeohashQueryRange {
	return GeohashQueryRange{
		Lower: geohash,
		Upper: geohash + "~",
	}
}
