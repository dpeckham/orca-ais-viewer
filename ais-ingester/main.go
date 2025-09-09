package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type StringUTCTime struct {
	time.Time
}

func (t *StringUTCTime) UnmarshalJSON(b []byte) (err error) {
    time, err := time.Parse(`"2006-01-02 15:04:05 -0700 UTC"`, string(b))
    if err != nil {
        return err
    }
	t.Time = time
    return
}

type SubscriptionMessage struct {
	APIKey             string        `json:"Apikey"`
	BoundingBoxes      [][][]float64 `json:"BoundingBoxes"`
	FilterMessageTypes []string      `json:"FilterMessageTypes"`
}

type GeoJSONPoint struct {
	Type        string    `json:"type" bson:"type"`
	Coordinates []float64 `json:"coordinates" bson:"coordinates"`
}

type AISMessage struct {
	Message     map[string]interface{} `json:"Message" bson:"message"`
	MessageType string                 `json:"MessageType" bson:"messageType"`
	MetaData    MetaData               `json:"MetaData" bson:"metadata"`
	GeoJSON     GeoJSONPoint           `bson:"geojson"`
}

type MetaData struct {
	MMSI      int       `json:"MMSI" bson:"mmsi"`
	ShipName  string    `json:"ShipName" bson:"shipName"`
	Latitude  float64   `json:"latitude" bson:"latitude"`
	Longitude float64   `json:"longitude" bson:"longitude"`
	TimeUTC   StringUTCTime    `json:"time_utc" bson:"timeUtc"`
}

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	ctx := context.Background()
	mongoURI := os.Getenv("MONGO_URI")

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("mongo connect:", err)
	}
	defer client.Disconnect(ctx)

	collection := client.Database("ais").Collection("positions")

	aisUrl := os.Getenv("AISSTREAM_URL")
	if aisUrl == "" {
		log.Fatal("AISSTREAM_URL environment variable is required")
	}
	log.Printf("connecting to %s", aisUrl)

	conn, _, err := websocket.DefaultDialer.Dial(aisUrl, nil)
	if err != nil {
		log.Fatal("dialer error:", err)
	}
	defer conn.Close()

	apiKey := os.Getenv("AISSTREAM_API_KEY")
	if apiKey == "" {
		log.Fatal("AISSTREAM_API_KEY environment variable is required")
	}

	subscription := SubscriptionMessage{
		APIKey: apiKey,
		BoundingBoxes: [][][]float64{
			//NOTE: order lat then lon.
			//NOTE: for demo purposes, New York to Boston
			{{40, -74.5}, {42, -68}},
		},
		FilterMessageTypes: []string{"PositionReport"},
	}

	if err := conn.WriteJSON(subscription); err != nil {
		log.Fatal("write subscription error:", err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("read error:", err)
				return
			}

			var aisMessage AISMessage
			if err := json.Unmarshal(message, &aisMessage); err != nil {
				log.Printf("unmarshal error: %v", err)
				continue
			}

			// Create GeoJSON Point from coordinates for geospatial querying
			aisMessage.GeoJSON = GeoJSONPoint{
				Type:        "Point",
				Coordinates: []float64{aisMessage.MetaData.Longitude, aisMessage.MetaData.Latitude},
			}

			// Upsert to MongoDB using MMSI as the key
			filter := bson.M{"metadata.mmsi": aisMessage.MetaData.MMSI}
			update := bson.M{"$set": aisMessage}
			opts := options.Update().SetUpsert(true)

			_, err = collection.UpdateOne(ctx, filter, update, opts)
			if err != nil {
				log.Printf("mongodb upsert error: %v", err)
				continue
			}

			log.Printf("Ingested a Postition Report for MMSI %v", aisMessage.MetaData.MMSI)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("interrupt")
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			}
			return
		}
	}
}
