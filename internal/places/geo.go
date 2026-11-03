package places

import "math"

const geohashAlphabet = "0123456789bcdefghjkmnpqrstuvwxyz"

// ConcertsPrecision is the geohash length Apple's concerts calls take: four
// characters, a cell about 39 km wide and 20 km high. Apple's web player
// asks with the same length.
const ConcertsPrecision = 4

// Geohash encodes a point as a geohash of the given length (1 to 12
// characters), the standard base-32 form with longitude first.
func Geohash(lat, lon float64, precision int) string {
	precision = min(max(precision, 1), 12)
	latMin, latMax := -90.0, 90.0
	lonMin, lonMax := -180.0, 180.0
	out := make([]byte, 0, precision)
	bit, idx := 0, 0
	even := true
	for len(out) < precision {
		if even {
			mid := (lonMin + lonMax) / 2
			if lon >= mid {
				idx = idx<<1 | 1
				lonMin = mid
			} else {
				idx <<= 1
				lonMax = mid
			}
		} else {
			mid := (latMin + latMax) / 2
			if lat >= mid {
				idx = idx<<1 | 1
				latMin = mid
			} else {
				idx <<= 1
				latMax = mid
			}
		}
		even = !even
		if bit++; bit == 5 {
			out = append(out, geohashAlphabet[idx])
			bit, idx = 0, 0
		}
	}
	return string(out)
}

// DistanceKm is the great-circle distance between two points in
// kilometres.
func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0088
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(a)))
}
