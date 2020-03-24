package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	adapter_library "github.com/clearblade/adapter-go-library"
	mqttTypes "github.com/clearblade/mqtt_parsing"
	"pack.ag/amqp"
)

const (
	adapterName = "amqp-1-0-adapter"
)

var (
	adapterSettings  *amqpAdapterSettings
	adapterConfig    *adapter_library.AdapterConfig
	amqpSession      *amqp.Session
	amqpSentMessages *sentMessages
)

type amqpAdapterSettings struct {
	BrokerAddress string   `json:"broker_address"`
	UseAnonAuth   bool     `json:"use_anon_auth"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	Queues        []string `json:"queues"`
}

type sentKey struct {
	Queue, Message string
}

type sentMessages struct {
	Mutex    *sync.Mutex
	Messages map[sentKey]int
}

func main() {
	err := adapter_library.ParseArguments(adapterName)
	if err != nil {
		log.Fatalf("[FATAL] Failed to parse arguments: %s\n", err.Error())
	}

	adapterConfig, err = adapter_library.Initialize()
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize: %s\n", err.Error())
	}

	// create map to store sent messages to AMQP broker, we check this map for every received message, if it's present we throw the message away
	// and don't send back to MQTT
	amqpSentMessages = &sentMessages{
		Mutex:    &sync.Mutex{},
		Messages: make(map[sentKey]int),
	}

	err = adapter_library.ConnectMQTT(adapterConfig.TopicRoot+"/outgoing/#", cbMessageHandler)

	adapterSettings = &amqpAdapterSettings{}
	err = json.Unmarshal([]byte(adapterConfig.AdapterSettings), adapterSettings)
	if err != nil {
		log.Fatalf("[FATAL] Failed to parse Adapter Settings: %s\n", err.Error())
	}

	// sub to AMQP topic specified in adapter settings
	go initializeAMQP()

	select {}
}

func cbMessageHandler(message *mqttTypes.Publish) {
	if len(message.Topic.Split) >= 3 {
		topicToUse := strings.Join(message.Topic.Split[2:], ".")
		amqpSentMessages.Mutex.Lock()
		amqpSentMessages.Messages[sentKey{topicToUse, string(message.Payload)}]++
		amqpSentMessages.Mutex.Unlock()
		ctx := context.Background()
		sender, err := amqpSession.NewSender(amqp.LinkTargetAddress(topicToUse))
		if err != nil {
			log.Printf("[ERROR] cbMessageHandler - Failed to create AMQP sender: %s", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = sender.Send(ctx, amqp.NewMessage(message.Payload))
		if err != nil {
			log.Printf("[ERROR] cbMessageHandler - Failed to send AMQP message: %s", err.Error())
		}
		sender.Close(ctx)
		cancel()
	} else {
		log.Printf("[ERROR] cbMessageHandler - Unexpected topic for message from MQTT Broker: %s\n", message.Topic.Whole)
	}
}

func initializeAMQP() {
	log.Println("[INFO] initializeAMQP - Connecting to AMQP broker...")
	// Create client
	client, err := amqp.Dial(adapterSettings.BrokerAddress, amqp.ConnSASLPlain(adapterSettings.Username, adapterSettings.Password))
	if err != nil {
		log.Fatal("[FATAL] initializeAMQP - Failed to dialing AMQP server:", err)
	}
	defer client.Close()
	log.Println("[DEBUG] initializeAMQP - AMQP client created")
	// Open a session and make it also available to the MQTT listener to be able to send messages to AMQP broker
	amqpSession, err = client.NewSession()
	if err != nil {
		log.Fatal("[FATAL] initializeAMQP - Failed to create AMQP session:", err)
	}
	log.Println("[DEBUG] initializeAMQP - AMQP session created")
	for _, q := range adapterSettings.Queues {
		go createAMQPReceiver(q, amqpSession)
	}
	// need to keep this func from returning to keep the AMQP broker connection up
	select {}
}

func createAMQPReceiver(queueName string, session *amqp.Session) {
	log.Println("[INFO] createAMQPReceiver - creating AMQP receiver for queue " + queueName)
	ctx := context.Background()
	receiver, err := session.NewReceiver(
		amqp.LinkSourceAddress(queueName),
		amqp.LinkCredit(10),
	)
	if err != nil {
		log.Fatal("[FATAL] createAMQPReceiver - Creating receiver link:", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		receiver.Close(ctx)
		cancel()
	}()

	for {
		// Receive next message
		msg, err := receiver.Receive(ctx)
		if err != nil {
			log.Fatal("[FATAL] createAMQPReceiver - Reading message from AMQP:", err)
		}

		// Accept message
		msg.Accept()
		amqpSentMessages.Mutex.Lock()
		key := sentKey{queueName, string(msg.GetData())}
		n := amqpSentMessages.Messages[key]
		if n == 1 {
			delete(amqpSentMessages.Messages, key)
			amqpSentMessages.Mutex.Unlock()
			log.Println("[DEBUG] - createAMQPReceiver - ignoring message because it came from Adapter")
			return
		} else if n > 1 {
			amqpSentMessages.Messages[key]--
			amqpSentMessages.Mutex.Unlock()
			log.Println("[DEBUG] - createAMQPReceived - ignoring message because it came from Adapter")
			return
		}
		amqpSentMessages.Mutex.Unlock()
		log.Printf("[DEBUG] createAMQPReceiver - AMQP message received queue: %s message: %s\n", queueName, msg.GetData())
		adapter_library.Publish(adapterConfig.TopicRoot+"/incoming/"+queueName, msg.GetData())
	}
}
