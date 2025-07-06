package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"context"
	"gochatapp/model"
	"gochatapp/pkg/redisrepo"

	"github.com/gorilla/websocket"
	"github.com/go-redis/redis/v8"
)


type Client struct {
	Conn     *websocket.Conn
	Username string
}

type Message struct {
	Type string     `json:"type"`
	User string     `json:"user,omitempty"`
	Chat model.Chat `json:"chat,omitempty"`
}

var clients = make(map[*Client]bool)
var redisClient *redis.Client
// We'll need to define an Upgrader
// this will require a Read and Write buffer size
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,

	// We'll need to check the origin of our connection
	// this will allow us to make requests from our React
	// development server to here.
	// For now, we'll do no checking and just allow any connection
	CheckOrigin: func(r *http.Request) bool { return true },
}

// define our WebSocket endpoint
func serveWs(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Host, r.URL.Query())

	// upgrade this connection to a WebSocket
	// connection
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}

	client := &Client{Conn: ws}
	// register client
	clients[client] = true
	fmt.Println("clients", len(clients), clients, ws.RemoteAddr())

	// listen indefinitely for new messages coming
	// through on our WebSocket connection
	go receiver(client)

}

// define a receiver which will listen for
// new messages being sent to our WebSocket
// endpoint
func receiver(client *Client) {
	defer func() {
		fmt.Println("exiting", client.Conn.RemoteAddr().String())
		client.Conn.Close()
		delete(clients, client)
	}()
	for {
		_, p, err := client.Conn.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			return
		}

		m := &Message{}
		if err := json.Unmarshal(p, m); err != nil {
			log.Println("unmarshal error:", err)
			continue
		}

		if m.Type == "bootup" {
			client.Username = m.User
			fmt.Println("client booted:", client.Username)
		} else {
			c := m.Chat
			c.Timestamp = time.Now().Unix()

			// Save to Redis (optional)
			id, err := redisrepo.CreateChat(&c)
			if err != nil {
				log.Println("error saving chat to Redis:", err)
				return
			}
			c.ID = id
			// receiver(): if chat.To user doesn't have From in contact list, add them
			_ = redisrepo.UpdateContactList(c.To, c.From)



			// Publish to Redis Pub/Sub channel
			msgJSON, _ := json.Marshal(c)
			err = redisClient.Publish(context.Background(), "chat_channel", msgJSON).Err()
			if err != nil {
				log.Println("publish error:", err)
			}
		}
	}
}

func startRedisSubscriber() {
	pubsub := redisClient.Subscribe(context.Background(), "chat_channel")
	ch := pubsub.Channel()//returns a go channel (producer)

	go func() {
		for msg := range ch { //consumer consumes the messages from the channel
			var chat model.Chat
			if err := json.Unmarshal([]byte(msg.Payload), &chat); err != nil {
				log.Println("unmarshal error in subscriber:", err)
				continue
			}

			// Deliver to all relevant WebSocket clients
			for client := range clients {
				if client.Username == chat.From || client.Username == chat.To {
					err := client.Conn.WriteJSON(&chat)
					if err != nil {
						log.Println("write error:", err)
						client.Conn.Close()
						delete(clients, client)
					}
				}
			}
		}
	}()
}


func setupRoutes() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Simple Server")
	})
	// map our `/ws` endpoint to the `serveWs` function
	http.HandleFunc("/ws", serveWs)
}

func StartWebsocketServer() {
	redisClient = redisrepo.InitialiseRedis() // Make sure this is accessible globally or pass it around
	defer redisClient.Close()

	startRedisSubscriber() // Start listening for published messages
	setupRoutes()

	log.Println("WebSocket server started on :8081")
	http.ListenAndServe(":8081", nil)
}

