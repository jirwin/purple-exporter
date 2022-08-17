package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/urfave/cli/v2"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	pm25aqi = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pm2_5_aqi",
			Help: "PM2.5 AQI",
		},
		[]string{
			"host",
			"channel",
		},
	)
	sensorId = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sensor_id",
			Help: "The sensor device ID",
		},
		[]string{
			"host",
			"sensor_id",
		},
	)
	version = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "version",
			Help: "Version",
		},
		[]string{
			"host",
			"version",
		},
	)
	temp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "temp_f",
		Help: "The current temperature (F)",
	}, []string{
		"host",
	})
	humidity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "humidity",
		Help: "The current humidity",
	}, []string{
		"host",
	})
	dewpoint = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dewpoint",
		Help: "The current dewpoint (F)",
	}, []string{
		"host",
	})
	pressure = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pressure",
		Help: "The current pressure in millibars",
	}, []string{
		"host",
	})
)

type PurpleAirData struct {
	SensorId    string  `json:"SensorId"`
	Place       string  `json:"place"`
	Version     string  `json:"version"`
	Temperature float64 `json:"current_temp_f"`
	Humidity    float64 `json:"current_humidity"`
	Dewpoint    float64 `json:"current_dewpoint_f"`
	Pressure    float64 `json:"pressure"`
	Pm25AqiA    float64 `json:"pm2.5_aqi"`
	Pm25AqiB    float64 `json:"pm2.5_aqi_b"`
}

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		&cli.StringSliceFlag{
			Name: "sensor-addr",
		},
		&cli.StringFlag{
			Name:  "listen",
			Value: "0.0.0.0:8080",
		},
	}
	app.Name = "purple-exporter"
	app.Version = "0.0.1"
	app.Action = run

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		os.Exit(1)
	}
}

func fetchStats(addrs []string) error {
	wg := sync.WaitGroup{}

	for _, addr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("http://%s/json", addr))
			if err != nil {
				log.Fatal(err)
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Fatal(err)
			}
			data := &PurpleAirData{}
			err = json.Unmarshal(body, data)
			if err != nil {
				log.Fatal(err)
			}

			sensorId.WithLabelValues(data.SensorId, data.SensorId).Set(1)
			version.WithLabelValues(data.SensorId, data.Version).Set(1)
			temp.WithLabelValues(data.SensorId).Set(data.Temperature)
			humidity.WithLabelValues(data.SensorId).Set(data.Humidity)
			dewpoint.WithLabelValues(data.SensorId).Set(data.Dewpoint)
			pressure.WithLabelValues(data.SensorId).Set(data.Pressure)
			pm25aqi.WithLabelValues(data.SensorId, "a").Set(data.Pm25AqiA)
			pm25aqi.WithLabelValues(data.SensorId, "b").Set(data.Pm25AqiB)
		}(addr)
	}
	wg.Wait()
	return nil
}

func run(c *cli.Context) error {
	sensorAddrs := c.StringSlice("sensor-addr")
	listenAddr := c.String("listen")

	if len(sensorAddrs) == 0 {
		return cli.NewExitError("At least one sensor address is required", -1)
	}

	ctx, canc := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(listenAddr, nil)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		interval := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-interval.C:
				err := fetchStats(sensorAddrs)
				if err != nil {
					fmt.Println(err.Error())
					<-time.After(time.Second * 1)
				}

			case <-ctx.Done():
				canc()
				wg.Done()
				return
			}
		}
	}()

	wg.Wait()

	fmt.Println("Shutting down")

	return nil
}
