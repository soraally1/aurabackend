// Package analytics provides predictive and analytical algorithms for environmental
// air pollution and traffic impact over a 7-day horizon.
package analytics

import (
	"fmt"
	"math"
	"time"

	"aurabackend/pkg/openmeteo"
	"aurabackend/pkg/zones"
)

// DailyImpact contains the evaluated risk, pollution, weather, and traffic for a specific day.
type DailyImpact struct {
	Date                 string   `json:"date"`                   // YYYY-MM-DD
	DayName              string   `json:"day_name"`               // Senin, Selasa, dll
	HealthRiskScore      float64  `json:"health_risk_score"`      // 0.0 - 100.0
	ImpactLevel          string   `json:"impact_level"`           // Aman, Waspada, Berbahaya
	PredictedPM25        float64  `json:"predicted_pm2_5"`        // ug/m3
	PredictedPM10        float64  `json:"predicted_pm10"`         // ug/m3
	PredictedCO          float64  `json:"predicted_co"`           // ug/m3
	PredictedAQI         int      `json:"predicted_aqi"`          // US AQI equivalent
	TrafficCongestionIdx float64  `json:"traffic_congestion_idx"` // 0.0 - 100.0
	TrafficSeverity      string   `json:"traffic_severity"`       // Lancar, Sedang, Padat, Sangat Padat
	Humidity             float64  `json:"humidity_percent"`       // %
	WindSpeedKmH         float64  `json:"wind_speed_kmh"`         // km/h
	WindDirectionDeg     float64  `json:"wind_direction_deg"`     // 0 - 360
	WindDirectionCompass string   `json:"wind_direction_compass"` // U, TL, T, TG, S, BD, B, BL
	TemperatureC         float64  `json:"temperature_c"`          // Suhu riil (misal 36.0)
	WeatherCondition     string   `json:"weather_condition"`      // "Berasap", "Cerah Berawan", dll
	DispersionCondition  string   `json:"dispersion_condition"`   // Dispersi Cepat, Normal, Terperangkap (Stagnan), Berasap
	PrecipitationMm      float64  `json:"precipitation_mm"`       // mm
	Summary              string   `json:"summary"`
	Recommendations      []string `json:"recommendations"`
}

// WeeklyImpactAnalysis represents the complete 7-day environmental impact forecast.
type WeeklyImpactAnalysis struct {
	LocationName        string        `json:"location_name"`
	Latitude            float64       `json:"latitude"`
	Longitude           float64       `json:"longitude"`
	ZoneCategory        string        `json:"zone_category,omitempty"`
	GeneratedAt         time.Time     `json:"generated_at"`
	OverallWeeklyStatus string        `json:"overall_weekly_status"`
	AverageRiskScore    float64       `json:"average_risk_score"`
	PeakRiskDay         string        `json:"peak_risk_day"`
	DailyForecast       []DailyImpact `json:"daily_forecast"`
	GeneralAdvice       []string      `json:"general_advice"`
}

// Analyze7DaysForecast evaluates a 7-day environmental and traffic impact.
func Analyze7DaysForecast(
	locationName string,
	lat, lon float64,
	zone *zones.Zone,
	env *openmeteo.EnvironmentalSnapshot,
) *WeeklyImpactAnalysis {
	now := time.Now().UTC()
	daysCount := 7
	dailyImpacts := make([]DailyImpact, 0, daysCount)

	totalRisk := 0.0
	maxRisk := -1.0
	peakDay := ""

	baseCongestion := 65.0
	historicalFactors := map[string]float64{
		"Monday":    1.25,
		"Tuesday":   1.15,
		"Wednesday": 1.10,
		"Thursday":  1.15,
		"Friday":    1.30,
		"Saturday":  0.85,
		"Sunday":    0.70,
	}

	zoneCategory := "Umum"
	if zone != nil {
		if zone.CongestionIndex > 0 {
			baseCongestion = zone.CongestionIndex
		}
		if len(zone.HistoricalTrafficFactor) > 0 {
			historicalFactors = zone.HistoricalTrafficFactor
		}
		if zone.Category != "" {
			zoneCategory = zone.Category
		}
	}

	weather := env.Weather.Daily

	for i := 0; i < daysCount; i++ {
		targetDate := now.AddDate(0, 0, i)
		dateStr := targetDate.Format("2006-01-02")
		weekdayEn := targetDate.Weekday().String()
		dayNameId := translateDayName(weekdayEn)

		// 1. Weather variables
		tempC := 36.0
		if len(weather.TemperatureMax) > i && weather.TemperatureMax[i] > 0 {
			tempC = math.Round(weather.TemperatureMax[i]*10) / 10
		}
		wCode := 0
		if len(weather.WeatherCode) > i {
			wCode = weather.WeatherCode[i]
		}
		humidity := 75.0
		if len(weather.RelativeHumidityMean) > i {
			humidity = weather.RelativeHumidityMean[i]
		}
		windSpeed := 10.0
		if len(weather.WindSpeedMax) > i {
			windSpeed = weather.WindSpeedMax[i]
		}
		windDir := 120.0
		if len(weather.WindDirectionDominant) > i {
			windDir = weather.WindDirectionDominant[i]
		}
		precip := 0.0
		if len(weather.PrecipitationSum) > i {
			precip = weather.PrecipitationSum[i]
		}

		// 2. Base air pollutants from forecast or snapshot
		basePM25, basePM10, baseCO, baseAQI := extractDayPollutants(i, env)

		// 3. Traffic congestion projection
		trafficFactor := historicalFactors[weekdayEn]
		if trafficFactor <= 0 {
			trafficFactor = 1.0
		}
		predCongestion := math.Min(100.0, math.Max(10.0, baseCongestion*trafficFactor))

		// 4. Atmospheric dispersion calculation
		// Higher wind speed -> rapid dispersion
		// Low wind speed + high humidity -> stagnant trapped air (smog)
		// Rain -> wet deposition / cleansing effect
		dispersionMultiplier, dispersionCond := calculateDispersion(windSpeed, humidity, precip)

		// 5. Traffic exhaust injection multiplier
		// Higher congestion emits more stagnant exhaust
		trafficEmissionMultiplier := 1.0 + ((predCongestion - 50.0) / 250.0)

		// 6. Effective PM2.5 calculation
		effectivePM25 := basePM25 * dispersionMultiplier * trafficEmissionMultiplier
		effectivePM10 := basePM10 * dispersionMultiplier * trafficEmissionMultiplier
		effectiveCO := baseCO * trafficEmissionMultiplier
		predAQI := int(math.Round(float64(baseAQI) * dispersionMultiplier))

		// Determine weather condition and smoke/haze status
		// WMO code 4 (smoke), 5 (haze), 45/48 (fog/depositing rime fog)
		// or when effective PM2.5 >= 35.0 or predicted AQI >= 100 with low rain (precip < 5.0)
		isSmoky := (wCode == 4 || wCode == 5 || wCode == 45 || wCode == 48) ||
			((effectivePM25 >= 35.0 || predAQI >= 100) && precip < 5.0)

		var weatherCondition string
		if isSmoky {
			weatherCondition = "Berasap"
			dispersionCond = "Berasap"
		} else if precip >= 10.0 {
			weatherCondition = "Hujan Lebat"
		} else if precip >= 2.0 {
			weatherCondition = "Hujan Ringan"
		} else if wCode == 1 || wCode == 2 {
			weatherCondition = "Cerah Berawan"
		} else if wCode == 3 {
			weatherCondition = "Berawan"
		} else if wCode >= 51 && wCode <= 67 {
			weatherCondition = "Hujan Ringan"
		} else if wCode >= 80 && wCode <= 99 {
			weatherCondition = "Hujan Petir"
		} else {
			weatherCondition = "Cerah"
		}

		// 7. Calculate Composite Health Risk Score (0 - 100)
		riskScore := calculateRiskScore(effectivePM25, baseAQI, predCongestion, humidity, windSpeed, precip)
		impactLevel := categorizeRiskLevel(riskScore)

		trafficSeverity := categorizeTraffic(predCongestion)
		recs := generateRecommendations(impactLevel, trafficSeverity, precip, effectivePM25)
		summary := fmt.Sprintf(
			"Kondisi %s dengan risiko kesehatan %.0f/100. Kemacetan %s (%0.0f%%) dan dispersi polutan %s.",
			impactLevel, riskScore, trafficSeverity, predCongestion, dispersionCond,
		)

		dailyImpact := DailyImpact{
			Date:                 dateStr,
			DayName:              dayNameId,
			HealthRiskScore:      math.Round(riskScore*10) / 10,
			ImpactLevel:          impactLevel,
			PredictedPM25:        math.Round(effectivePM25*10) / 10,
			PredictedPM10:        math.Round(effectivePM10*10) / 10,
			PredictedCO:          math.Round(effectiveCO*10) / 10,
			PredictedAQI:         predAQI,
			TrafficCongestionIdx: math.Round(predCongestion*10) / 10,
			TrafficSeverity:      trafficSeverity,
			Humidity:             humidity,
			WindSpeedKmH:         windSpeed,
			WindDirectionDeg:     windDir,
			WindDirectionCompass: degreeToCompass(windDir),
			TemperatureC:         tempC,
			WeatherCondition:     weatherCondition,
			DispersionCondition:  dispersionCond,
			PrecipitationMm:      precip,
			Summary:              summary,
			Recommendations:      recs,
		}

		dailyImpacts = append(dailyImpacts, dailyImpact)
		totalRisk += riskScore
		if riskScore > maxRisk {
			maxRisk = riskScore
			peakDay = fmt.Sprintf("%s (%s)", dayNameId, dateStr)
		}
	}

	avgRisk := totalRisk / float64(daysCount)
	overallStatus := categorizeRiskLevel(avgRisk)

	return &WeeklyImpactAnalysis{
		LocationName:        locationName,
		Latitude:            lat,
		Longitude:           lon,
		ZoneCategory:        zoneCategory,
		GeneratedAt:         now,
		OverallWeeklyStatus: overallStatus,
		AverageRiskScore:    math.Round(avgRisk*10) / 10,
		PeakRiskDay:         peakDay,
		DailyForecast:       dailyImpacts,
		GeneralAdvice: []string{
			"Pantau indeks risiko setiap pagi sebelum beraktivitas di koridor jalan raya.",
			"Aktifkan sirkulasi udara internal (recirculate) pada kendaraan saat melintasi zona kemacetan pekat.",
			"Kelompok rentan (anak-anak, lansia, penderita asma) disarankan membatasi paparan luar ruang di hari berstatus Waspada/Berbahaya.",
			"Manfaatkan transportasi umum berbasis rel atau rute alternatif non-koridor macet untuk menekan paparan gas emisi.",
		},
	}
}

func calculateDispersion(windSpeed, humidity, precip float64) (float64, string) {
	multiplier := 1.0

	// Wind speed influence
	if windSpeed >= 16.0 {
		multiplier *= 0.80 // Angin kencang mengurai polutan
	} else if windSpeed < 6.0 {
		multiplier *= 1.25 // Angin tenang menjebak polutan lokal
	}

	// Humidity influence (aerosol hygroscopic growth)
	if humidity >= 82.0 {
		multiplier *= 1.15
	} else if humidity <= 55.0 {
		multiplier *= 0.95
	}

	// Rain wash-out
	if precip >= 5.0 {
		multiplier *= 0.75 // Hujan membersihkan partikulat dari atmosfer
	}

	// Dispersion label
	cond := "Normal"
	if multiplier <= 0.85 {
		cond = "Dispersi Cepat (Udara Mengalir Cepat)"
	} else if multiplier >= 1.20 {
		cond = "Terperangkap (Stagnan & Berasap)"
	}

	return multiplier, cond
}

func calculateRiskScore(pm25 float64, aqi int, congestion, humidity, windSpeed, precip float64) float64 {
	// 1. PM2.5 Component (0 - 45 pts): WHO guideline is 15 ug/m3, 55+ ug/m3 is unhealthy
	pm25Score := (pm25 / 75.0) * 45.0
	if pm25Score > 45.0 {
		pm25Score = 45.0
	}

	// 2. Congestion Component (0 - 30 pts)
	congestionScore := (congestion / 100.0) * 30.0

	// 3. Stagnation / Weather Inversion Component (0 - 15 pts)
	stagnationScore := 7.5
	if windSpeed < 7.0 && humidity > 80.0 {
		stagnationScore = 15.0
	} else if windSpeed > 15.0 || precip > 5.0 {
		stagnationScore = 2.0
	}

	// 4. Baseline AQI Component (0 - 10 pts)
	aqiScore := (float64(aqi) / 200.0) * 10.0
	if aqiScore > 10.0 {
		aqiScore = 10.0
	}

	total := pm25Score + congestionScore + stagnationScore + aqiScore
	return math.Min(100.0, math.Max(5.0, total))
}

func categorizeRiskLevel(score float64) string {
	switch {
	case score < 38.0:
		return "Aman"
	case score < 68.0:
		return "Waspada"
	default:
		return "Berbahaya"
	}
}

func categorizeTraffic(idx float64) string {
	switch {
	case idx < 45.0:
		return "Lancar"
	case idx < 68.0:
		return "Sedang"
	case idx < 83.0:
		return "Padat"
	default:
		return "Sangat Padat"
	}
}

func extractDayPollutants(dayIndex int, env *openmeteo.EnvironmentalSnapshot) (float64, float64, float64, int) {
	// If hourly data is available, aggregate 24 hours for the day
	hourly := env.HourlyAQ
	startHour := dayIndex * 24
	endHour := startHour + 24

	if len(hourly.PM25) >= endHour {
		var sumPM25, sumPM10, sumCO float64
		var maxAQI int
		count := 0
		for h := startHour; h < endHour; h++ {
			sumPM25 += hourly.PM25[h]
			sumPM10 += hourly.PM10[h]
			sumCO += hourly.CarbonMonoxide[h]
			if hourly.USAQI[h] > maxAQI {
				maxAQI = hourly.USAQI[h]
			}
			count++
		}
		if count > 0 {
			return sumPM25 / float64(count), sumPM10 / float64(count), sumCO / float64(count), maxAQI
		}
	}

	// Fallback to current air quality
	return env.AirQuality.PM25, env.AirQuality.PM10, env.AirQuality.CarbonMonoxide, env.AirQuality.USAQI
}

func generateRecommendations(riskLevel, trafficSeverity string, precip, pm25 float64) []string {
	var recs []string

	if riskLevel == "Berbahaya" {
		recs = append(recs, "Wajib kenakan masker partikulat (N95 / KF94) saat berkendara atau beraktivitas di luar.")
		recs = append(recs, "Hindari olahraga atau jogging di dekat koridor jalan utama.")
		recs = append(recs, "Rencanakan rute alternatif sebelum jam puncak kemacetan.")
	} else if riskLevel == "Waspada" {
		recs = append(recs, "Kelompok sensitif dianjurkan mengenakan masker medis atau masker anti-polusi.")
		recs = append(recs, "Tutup jendela kendaraan dan nyalakan filter AC kabin.")
	} else {
		recs = append(recs, "Kualitas udara relatif aman untuk sebagian besar aktivitas luar ruangan.")
		recs = append(recs, "Lalu lintas terkendali, perjalanan dapat dilakukan dengan nyaman.")
	}

	if trafficSeverity == "Sangat Padat" || trafficSeverity == "Padat" {
		recs = append(recs, "Pertimbangkan berangkat 30 menit lebih awal untuk menghindari titik akumulasi gas buang kendaraan.")
	}

	if precip > 5.0 {
		recs = append(recs, "Potensi hujan membantu mengendapkan debu polusi; antisipasi perlambatan laju lalu lintas.")
	}

	return recs
}

func translateDayName(enDay string) string {
	switch enDay {
	case "Monday":
		return "Senin"
	case "Tuesday":
		return "Selasa"
	case "Wednesday":
		return "Rabu"
	case "Thursday":
		return "Kamis"
	case "Friday":
		return "Jumat"
	case "Saturday":
		return "Sabtu"
	case "Sunday":
		return "Minggu"
	default:
		return enDay
	}
}

func degreeToCompass(deg float64) string {
	val := int((deg / 45.0) + 0.5)
	compass := []string{"Utara", "Timur Laut", "Timur", "Tenggara", "Selatan", "Barat Daya", "Barat", "Barat Laut"}
	return compass[val%8]
}
