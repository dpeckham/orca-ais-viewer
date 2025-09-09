package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SubscribeMessage struct {
	Type        string      `json:"type"`
	BoundingBox [][]float64 `json:"boundingBox"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	mongoClient := connectToMongoDB()
	defer mongoClient.Disconnect(context.Background())

	http.HandleFunc("/ais", handleWebSocket(mongoClient))

	log.Println("WebSocket server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}


func handleWebSocket(mongoClient *mongo.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()

		log.Printf("New WebSocket connection established from %s", r.RemoteAddr)

		// Wait for subscription message
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Failed to read subscription message: %v", err)
			return
		}

		var subscribeMsg SubscribeMessage
		if err := json.Unmarshal(message, &subscribeMsg); err != nil {
			log.Printf("Failed to unmarshal subscription message: %v", err)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"Invalid subscription message format"}`))
			return
		}

		// Validate subscription message
		if subscribeMsg.Type != "subscribe" {
			log.Printf("Invalid message type: %s", subscribeMsg.Type)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"First message must be a subscribe message"}`))
			return
		}

		if len(subscribeMsg.BoundingBox) != 2 || len(subscribeMsg.BoundingBox[0]) != 2 || len(subscribeMsg.BoundingBox[1]) != 2 {
			log.Printf("Invalid bounding box format")
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"Bounding box must contain exactly 2 coordinate pairs [lon, lat]"}`))
			return
		}

		log.Printf("Client subscribed with bounding box: [[%.6f, %.6f], [%.6f, %.6f]]",
			subscribeMsg.BoundingBox[0][0], subscribeMsg.BoundingBox[0][1],
			subscribeMsg.BoundingBox[1][0], subscribeMsg.BoundingBox[1][1])

		// Send confirmation
		conn.WriteMessage(websocket.TextMessage, []byte(`{"status":"subscribed"}`))

		// Start a ticker to send time every second
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		// Channel to handle connection close
		done := make(chan struct{})

		// Goroutine to handle additional incoming messages
		go func() {
			defer close(done)
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					log.Printf("Read error: %v", err)
					return
				}
				log.Printf("Received message: %s", message)
			}
		}()

		lon1, lat1 := subscribeMsg.BoundingBox[0][0], subscribeMsg.BoundingBox[0][1]
		lon2, lat2 := subscribeMsg.BoundingBox[1][0], subscribeMsg.BoundingBox[1][1]

		polygon := bson.M{
			"type": "Polygon",
			"coordinates": []interface{}{
				[]interface{}{
					[]float64{lon1, lat1}, // bottom-left
					[]float64{lon2, lat1}, // bottom-right
					[]float64{lon2, lat2}, // top-right
					[]float64{lon1, lat2}, // top-left
					[]float64{lon1, lat1}, // close the polygon
				},
			},
		}

		// Main loop to send query results
		for {
			select {
			case <-done:
				log.Printf("WebSocket connection closed for %s", r.RemoteAddr)
				return
			case <-ticker.C:
				// Query MongoDB for positions within the bounding box
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

				database := mongoClient.Database("ais")
				collection := database.Collection("positions")

				// Create the $geoWithin query with time filter for last 2 minutes
				twoMinutesAgo := time.Now().UTC().Add(-2 * time.Minute)
				filter := bson.M{
					"geojson": bson.M{
						"$geoWithin": bson.M{
							"$geometry": polygon,
						},
					},
					"metadata.timeUtc.time": bson.M{
						"$gte": twoMinutesAgo,
					},
				}

				cursor, err := collection.Find(ctx, filter)
				cancel()

				if err != nil {
					log.Printf("MongoDB query error: %v", err)
					continue
				}

				var results []bson.M
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				err = cursor.All(ctx2, &results)
				cancel2()
				cursor.Close(context.Background())

				if err != nil {
					log.Printf("Error reading query results: %v", err)
					continue
				}

				// Convert results to GeoJSON FeatureCollection
				features := make([]interface{}, 0, len(results))

				for _, result := range results {
					// Extract coordinates from existing geojson field
					var coordinates []float64
					if geojsonField, ok := result["geojson"].(bson.M); ok {
						if coordsField, ok := geojsonField["coordinates"].(bson.A); ok && len(coordsField) == 2 {
							if lon, ok := coordsField[0].(float64); ok {
								if lat, ok := coordsField[1].(float64); ok {
									coordinates = []float64{lon, lat}
								}
							}
						}
					}

					// Skip if we couldn't extract coordinates
					if len(coordinates) == 0 {
						continue
					}

					var heading interface{}
					if messageField, ok := result["message"].(bson.M); ok {
						if positionReportField, ok := messageField["PositionReport"].(bson.M); ok {
							if trueHeading, exists := positionReportField["TrueHeading"]; exists {
								heading = trueHeading
							}
						}
					}

					// Extract other useful metadata
					properties := map[string]interface{}{}
					if heading != nil {
						properties["heading"] = heading
					}

					// Add MMSI and ship name if available
					if metadataField, ok := result["metadata"].(bson.M); ok {
						if mmsi, exists := metadataField["mmsi"]; exists {
							properties["mmsi"] = mmsi
						}
						if shipName, exists := metadataField["shipName"]; exists {
							properties["shipName"] = shipName
						}
						if timeUtc, exists := metadataField["timeUtc"]; exists {
							properties["timeUtc"] = timeUtc
						}
					}

					feature := map[string]interface{}{
						"type": "Feature",
						"geometry": map[string]interface{}{
							"type":        "Point",
							"coordinates": coordinates,
						},
						"properties": properties,
					}

					features = append(features, feature)
				}

				// Create GeoJSON FeatureCollection
				response := map[string]interface{}{
					"type":     "FeatureCollection",
					"features": features,
				}

				jsonData, err := json.Marshal(response)
				if err != nil {
					log.Printf("Error marshaling response: %v", err)
					continue
				}

				err = conn.WriteMessage(websocket.TextMessage, jsonData)
				if err != nil {
					log.Printf("Write error: %v", err)
					return
				}
			}
		}
	}
}

func connectToMongoDB() *mongo.Client {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("MONGO_URI environment variable is required")
	}

	clientOptions := options.Client().ApplyURI(mongoURI)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	log.Println("Connected to MongoDB")
	return client
}


