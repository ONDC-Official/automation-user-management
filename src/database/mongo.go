package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var DB *mongo.Database

// Connect establishes a connection to MongoDB
func Connect(uri string, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return err
	}

	// Ping the database to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return err
	}

	DB = client.Database(dbName)
	fmt.Println("Connected to MongoDB successfully")
	return nil
}

// GetCollection is a helper to retrieve a collection handle
func GetCollection(collectionName string) *mongo.Collection {
	return DB.Collection(collectionName)
}

// CreateOne inserts a single document
func CreateOne(collectionName string, doc interface{}) (*mongo.InsertOneResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return GetCollection(collectionName).InsertOne(ctx, doc)
}

// FindOne finds a single document and decodes it into the result interface
func FindOne(collectionName string, filter interface{}, result interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return GetCollection(collectionName).FindOne(ctx, filter).Decode(result)
}

// FindMany finds multiple documents and decodes them into the results interface (slice pointer)
func FindMany(collectionName string, filter interface{}, results interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := GetCollection(collectionName).Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	return cursor.All(ctx, results)
}

// UpdateOne updates a single document
func UpdateOne(collectionName string, filter interface{}, update interface{}) (*mongo.UpdateResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return GetCollection(collectionName).UpdateOne(ctx, filter, update)
}

// DeleteOne deletes a single document
func DeleteOne(collectionName string, filter interface{}) (*mongo.DeleteResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return GetCollection(collectionName).DeleteOne(ctx, filter)
}

// Aggregate finds documents using a pipeline and decodes them into the results interface
func Aggregate(collectionName string, pipeline interface{}, results interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := GetCollection(collectionName).Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	return cursor.All(ctx, results)
}

// UpsertOne updates a single document, inserting it if it does not exist.
func UpsertOne(collectionName string, filter interface{}, update interface{}) (*mongo.UpdateResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := options.Update().SetUpsert(true)
	return GetCollection(collectionName).UpdateOne(ctx, filter, update, opts)
}

// EnsureUniqueIndex creates a unique index on the given field for a collection.
func EnsureUniqueIndex(collectionName string, fieldName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := GetCollection(collectionName)
	indexModel := mongo.IndexModel{
		Keys:    map[string]interface{}{fieldName: 1},
		Options: options.Index().SetUnique(true),
	}

	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	return err
}

// EnsureTTLIndex creates a TTL index on a field that automatically deletes documents after a certain duration.
func EnsureTTLIndex(collectionName string, fieldName string, expireAfterSeconds int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := GetCollection(collectionName)
	indexModel := mongo.IndexModel{
		Keys:    map[string]interface{}{fieldName: 1},
		Options: options.Index().SetExpireAfterSeconds(expireAfterSeconds),
	}

	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	return err
}
