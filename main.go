package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/bson/objectid"
	"github.com/mongodb/mongo-go-driver/mongo"
	"log"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type key string

const (
	hostKey     = key("hostKey")
	usernameKey = key("usernameKey")
	passwordKey = key("passwordKey")
	databaseKey = key("databaseKey")
)

type command int

const (
	createCmd command = iota
	readCmd
	updateCmd
	deleteCmd
	exitCmd
)

const displayLayout = `01/02/2006`

const menu = `
To-Do List
=======================
0) Create new To-do
1) List To-dos
2) Update To-do (by ID)
3) Delete To-do (by ID)
4) Exit
`

const collName = "todos"

type todo struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"createdAt"`
	ModifiedAt  time.Time `json:"modifiedAt"`
	Task        string    `json:"task"`
}

func (t *todo) String() string {
	return t.Task
}

func createTask(ctx context.Context, db *mongo.Database) (string, error) {
	fmt.Print("what is the task that you have to do? ")
	var task string
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		if err := input.Err(); err != nil {
			return "", fmt.Errorf("createTask: couldn't get task from command line: %v", err)
		}
		task = input.Text()
		if len(strings.Trim(task, " ")) == 0 {
			return "", errors.New("createTask: can't create a to-do item with no task")
		}
		break
	}
	now := time.Now()
	t := todo{
		CreatedAt:   now,
		ModifiedAt:  now,
		Task:        task,
	}
	res, err := db.Collection(collName).InsertOne(ctx, bson.NewDocument(
		bson.EC.String("task", t.Task),
		bson.EC.DateTime("createdAt", timeMillis(t.CreatedAt)),
		bson.EC.DateTime("modifiedAt", timeMillis(t.ModifiedAt)),
	))
	if err != nil {
		return "", fmt.Errorf("createTask: task for to-do list couldn't be created: %v", err)
	}
	id := res.InsertedID.(objectid.ObjectID)
	return id.Hex(), nil
}

func timeMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func readTasks(ctx context.Context, db *mongo.Database) error {
	c, err := db.Collection(collName).Find(ctx, nil)
	if err != nil {
		return fmt.Errorf("readTasks: couldn't list all to-dos: %v", err)
	}
	defer c.Close(ctx)
	tw := tabwriter.NewWriter(os.Stdout, 24, 2, 4, ' ', tabwriter.TabIndent)
	fmt.Fprintln(tw, "ID\tCreated At\tModified At\tTask\t")
	for c.Next(ctx) {
		elem := bson.NewDocument()
		if err = c.Decode(elem); err != nil {
			return fmt.Errorf("readTasks: couldn't make to-do item ready for display: %v", err)
		}
		t := todo{
			ID:          elem.Lookup("_id").ObjectID().Hex(),
			CreatedAt:   elem.Lookup("createdAt").DateTime().UTC(),
			ModifiedAt:  elem.Lookup("modifiedAt").DateTime().UTC(),
			Task:        elem.Lookup("task").StringValue(),
		}
		output := fmt.Sprintf("%s\t%s\t%s\t%s\t",
			t.ID,
			formatForDisplay(t.CreatedAt),
			formatForDisplay(t.ModifiedAt),
			t.Task,
		)
		fmt.Fprintln(tw, output)
		if err = tw.Flush(); err != nil {
			return fmt.Errorf("readTasks: all data for the to-do couldn't be printed: %v", err)
		}
	}
	if err = c.Err(); err != nil {
		return fmt.Errorf("readTasks: all to-do items couldn't be listed: %v", err)
	}
	return nil
}

func formatForDisplay(t time.Time) string {
	return t.Format(displayLayout)
}

func updateTask(ctx context.Context, db *mongo.Database) (string, error) {
	fmt.Print("task ID: ")
	var id string
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		if err := input.Err(); err != nil {
			return "", fmt.Errorf("updateTask: couldn't readTasks task ID from command line: %v", err)
		}
		id = input.Text()
		if len(strings.Trim(id, " ")) == 0 {
			return "", errors.New("updateTask: can't updateTask a to-do item with no task ID")
		}
		break
	}
	objectIDS, err := objectid.FromHex(id)
	if err != nil {
		return "", fmt.Errorf("updateTask: couldn't get object ID from given input: %v", err)
	}
	var t todo
	idDoc := bson.NewDocument(bson.EC.ObjectID("_id", objectIDS))
	err = db.Collection(collName).FindOne(ctx, idDoc).Decode(&t)
	if err != nil {
		return "", fmt.Errorf("updateTask: couldn't decode task from db: %v", err)
	}
	var task string
	fmt.Println("old task:", t.Task)
	fmt.Print("updated task: ")
	for input.Scan() {
		if err := input.Err(); err != nil {
			return "", fmt.Errorf("updateTask: couldn't read task from command line: %v", err)
		}
		task = input.Text()
		if len(strings.Trim(task, " ")) == 0 {
			return "", errors.New("updateTask: can't update a to-do item with no task")
		}
		t.Task = task
		break
	}
	_, err = db.Collection(collName).UpdateOne(
		ctx,
		idDoc,
		bson.NewDocument(
			bson.EC.SubDocumentFromElements("$set",
				bson.EC.String("task", t.Task),
			),
			bson.EC.SubDocumentFromElements("$currentDate",
				bson.EC.Boolean("modifiedAt", true),
			),
		),
	)
	if err != nil {
		return "", fmt.Errorf("updateTask: task for to-do list couldn't be created: %v", err)
	}
	return id, nil
}

func deleteTask(ctx context.Context, db *mongo.Database) (int64, error) {
	fmt.Print("task ID: ")
	var id string
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		if err := input.Err(); err != nil {
			return 0, fmt.Errorf("deleteTask: couldn't get to-do ID from command line: %v", err)
		}
		id = input.Text()
		if len(strings.Trim(id, " ")) == 0 {
			return 0, errors.New("deleteTask: can't delete a to-do item with no task ID")
		}
		break
	}
	objectIDS, err := objectid.FromHex(id)
	if err != nil {
		return 0, fmt.Errorf("deleteTask: couldn't convert to-do ID from input: %v", err)
	}
	idDoc := bson.NewDocument(bson.EC.ObjectID("_id", objectIDS))
	res, err := db.Collection(collName).DeleteOne(ctx, idDoc)
	if err != nil {
		return 0, fmt.Errorf("deleteTask: couldn't delete to-do from db: %v", err)
	}
	return res.DeletedCount, nil
}

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = context.WithValue(ctx, hostKey, os.Getenv("TODO_MONGO_HOST"))
	ctx = context.WithValue(ctx, usernameKey, os.Getenv("TODO_MONGO_USERNAME"))
	ctx = context.WithValue(ctx, passwordKey, os.Getenv("TODO_MONGO_PASSWORD"))
	ctx = context.WithValue(ctx, databaseKey, os.Getenv("TODO_MONGO_DATABASE"))
	db, err := configDB(ctx)
	if err != nil {
		log.Fatalf("todo: database configuration failed: %v", err)
	}
	err = run(ctx, db)
	if err != nil {
		log.Fatalf("todo: command number processing failed: %v", err)
	}
}
func run(ctx context.Context, db *mongo.Database) error {
	for {
		fmt.Print(menu)
		fmt.Print("\nEnter a command number: ")
		input := bufio.NewScanner(os.Stdin)
		var cmd command
		var err error
		for input.Scan() {
			if err = input.Err(); err != nil {
				return fmt.Errorf("run: command number couldn't be readCmd: %v", err)
			}
			i, err := strconv.Atoi(input.Text())
			cmd = command(i)
			if err != nil {
				return fmt.Errorf("run: invalid command number %d given: %v", cmd, err)
			}
			break
		}
		err = execCmd(ctx, db, cmd)
		if err != nil {
			return fmt.Errorf("run: couldn't execute the previous command: %v", err)
		}
	}
	return nil
}

func execCmd(ctx context.Context, db *mongo.Database, cmd command) error {
	switch command(cmd) {
	case createCmd:
		id, err := createTask(ctx, db)
		if err != nil {
			return fmt.Errorf("execCmd: to-do creation failed: %v", err)
		}
		fmt.Println("created a new to-do with ID:", id)
		break
	case readCmd:
		err := readTasks(ctx, db)
		if err != nil {
			return fmt.Errorf("execCmd: listing to-dos failed: %v", err)
		}
		break
	case updateCmd:
		id, err := updateTask(ctx, db)
		if err != nil {
			return fmt.Errorf("execCmd: to-do update task failed: %v", err)
		}
		fmt.Println("updated existing to-do with ID:", id)
		break
	case deleteCmd:
		id, err := deleteTask(ctx, db)
		if err != nil {
			return fmt.Errorf("execCmd: to-do deletion failed: %v", err)
		}
		fmt.Printf("deleted %d to-do", id)
		break
	case exitCmd:
		fmt.Println("Good-bye!")
		os.Exit(0)
		break
	}
	return run(ctx, db)
}

func configDB(ctx context.Context) (*mongo.Database, error) {
	uri := fmt.Sprintf(`mongodb://%s:%s@%s/%s`,
		ctx.Value(usernameKey),
		ctx.Value(passwordKey),
		ctx.Value(hostKey),
		ctx.Value(databaseKey),
	)
	client, err := mongo.NewClient(uri)
	if err != nil {
		return nil, fmt.Errorf("todo: couldn't connect to mongo: %v", err)
	}
	err = client.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("todo: mongo client couldn't connect with background context: %v", err)
	}
	todoDB := client.Database("todo")
	return todoDB, nil
}
