package qdrant

import (
	"context"
	"fmt"
	"log"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
)

type Client struct {
	client *qdrant.Client
}

type Product struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Price       float64                `json:"price"`
	ImageURL    string                 `json:"image_url"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type SearchResult struct {
	Product *Product `json:"product"`
	Score   float64  `json:"score"`
}

type Config struct {
	Host       string
	Port       int
	APIKey     string
	UseTLS     bool
	Collection string
}

// MaxGrpcMessageSize is the maximum gRPC message size for Qdrant responses (16 MB)
const MaxGrpcMessageSize = 16 * 1024 * 1024

func NewClient(cfg Config) (*Client, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		APIKey: cfg.APIKey,
		UseTLS: cfg.UseTLS,
		GrpcOptions: []grpc.DialOption{
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxGrpcMessageSize)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	return &Client{client: client}, nil
}
func (c *Client) SearchProducts(ctx context.Context, collectionName string, queryVector []float32, limit uint64, scoreThreshold float32) ([]SearchResult, error) {
	response, err := c.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &scoreThreshold,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search products in collection %s: %w", collectionName, err)
	}

	log.Printf("Search products response: found %d results", len(response))

	results := make([]SearchResult, len(response))
	for i, point := range response {
		log.Printf("Processing point %d with %d payload fields", i, len(point.Payload))

		product := &Product{
			ID:          point.Payload["id"].GetStringValue(),
			Name:        point.Payload["name"].GetStringValue(),
			Description: point.Payload["description"].GetStringValue(),
			Category:    point.Payload["category"].GetStringValue(),
			Price:       point.Payload["price"].GetDoubleValue(),
			ImageURL:    point.Payload["image_urls"].GetStringValue(),
			Metadata:    make(map[string]interface{}),
		}

		for key, value := range point.Payload {
			if key == "id" || key == "name" || key == "description" || key == "category" || key == "price" || key == "image_urls" {
				continue
			}

			// Handle the special "metadata" field that contains nested structure
			if key == "metadata" {
				if value.GetStructValue() != nil {
					// Extract fields from the nested metadata structure
					structValue := value.GetStructValue()

					if structValue != nil && structValue.Fields != nil {
						for nestedKey, nestedValue := range structValue.Fields {
							switch nestedValue.Kind.(type) {
							case *qdrant.Value_StringValue:
								stringVal := nestedValue.GetStringValue()
								product.Metadata[nestedKey] = stringVal
							case *qdrant.Value_DoubleValue:
								doubleVal := nestedValue.GetDoubleValue()
								product.Metadata[nestedKey] = doubleVal
							case *qdrant.Value_BoolValue:
								boolVal := nestedValue.GetBoolValue()
								product.Metadata[nestedKey] = boolVal
							case *qdrant.Value_IntegerValue:
								intVal := nestedValue.GetIntegerValue()
								product.Metadata[nestedKey] = intVal
							default:
								log.Printf("Unknown nested metadata type for %s: %T", nestedKey, nestedValue.Kind)
							}
						}
					}
				} else {
					log.Printf("Metadata field exists but struct value is nil")
				}
			} else {
				switch value.Kind.(type) {
				case *qdrant.Value_StringValue:
					stringVal := value.GetStringValue()
					product.Metadata[key] = stringVal
				case *qdrant.Value_DoubleValue:
					doubleVal := value.GetDoubleValue()
					product.Metadata[key] = doubleVal
				case *qdrant.Value_BoolValue:
					boolVal := value.GetBoolValue()
					product.Metadata[key] = boolVal
				case *qdrant.Value_IntegerValue:
					intVal := value.GetIntegerValue()
					product.Metadata[key] = intVal
				default:
					log.Printf("Unknown direct metadata type for %s: %T", key, value.Kind)
				}
			}
		}

		results[i] = SearchResult{
			Product: product,
			Score:   float64(point.Score),
		}
	}

	return results, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}
