// Copyright © 2016 The Things Network
// Use of this source code is governed by the MIT license that can be found in the LICENSE file.

package mqtt

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/TheThingsNetwork/ttn/utils/testing"
	"github.com/apex/log"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	. "github.com/smartystreets/assertions"
)

var host string

func init() {
	host = os.Getenv("MQTT_HOST")
	if host == "" {
		host = "localhost"
	}
}

func waitForOK(token Token, a *Assertion) {
	success := token.WaitTimeout(100 * time.Millisecond)
	a.So(success, ShouldBeTrue)
	a.So(token.Error(), ShouldBeNil)
}

func TestToken(t *testing.T) {
	a := New(t)

	okToken := newToken()
	go func() {
		time.Sleep(1 * time.Millisecond)
		okToken.flowComplete()
	}()
	okToken.Wait()
	a.So(okToken.Error(), ShouldBeNil)

	failToken := newToken()
	go func() {
		time.Sleep(1 * time.Millisecond)
		failToken.err = errors.New("Err")
		failToken.flowComplete()
	}()
	failToken.Wait()
	a.So(failToken.Error(), ShouldNotBeNil)

	timeoutToken := newToken()
	timeoutTokenDone := timeoutToken.WaitTimeout(5 * time.Millisecond)
	a.So(timeoutTokenDone, ShouldBeFalse)
}

func TestSimpleToken(t *testing.T) {
	a := New(t)

	okToken := simpleToken{}
	okToken.Wait()
	a.So(okToken.Error(), ShouldBeNil)

	failToken := simpleToken{fmt.Errorf("Err")}
	failToken.Wait()
	a.So(failToken.Error(), ShouldNotBeNil)
}

func TestNewClient(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	a.So(c.(*DefaultClient).mqtt, ShouldNotBeNil)
}

func TestConnect(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	err := c.Connect()
	defer c.Disconnect()
	a.So(err, ShouldBeNil)

	// Connecting while already connected should not change anything
	err = c.Connect()
	defer c.Disconnect()
	a.So(err, ShouldBeNil)
}

func TestConnectInvalidAddress(t *testing.T) {
	a := New(t)
	ConnectRetries = 2
	ConnectRetryDelay = 50 * time.Millisecond
	c := NewClient(GetLogger(t, "Test"), "test", "", "", "tcp://localhost:18830") // No MQTT on 18830
	err := c.Connect()
	defer c.Disconnect()
	a.So(err, ShouldNotBeNil)
}

func TestConnectInvalidCredentials(t *testing.T) {
	t.Skipf("Need authenticated MQTT for TestConnectInvalidCredentials - Skipping")
}

func TestIsConnected(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))

	a.So(c.IsConnected(), ShouldBeFalse)

	c.Connect()
	defer c.Disconnect()

	a.So(c.IsConnected(), ShouldBeTrue)
}

func TestDisconnect(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))

	// Disconnecting when not connected should not change anything
	c.Disconnect()
	a.So(c.IsConnected(), ShouldBeFalse)

	c.Connect()
	defer c.Disconnect()
	c.Disconnect()

	a.So(c.IsConnected(), ShouldBeFalse)
}

func TestRandomTopicPublish(t *testing.T) {
	a := New(t)
	ctx := GetLogger(t, "TestRandomTopicPublish")

	c := NewClient(ctx, "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	subToken := c.(*DefaultClient).mqtt.Subscribe("randomtopic", QoS, nil)
	waitForOK(subToken, a)
	pubToken := c.(*DefaultClient).mqtt.Publish("randomtopic", QoS, false, []byte{0x00})
	waitForOK(pubToken, a)

	<-time.After(50 * time.Millisecond)

	ctx.Info("This test should have printed one message.")
}

// Uplink pub/sub

func TestPublishUplink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	dataUp := UplinkMessage{
		AppID:   "someid",
		DevID:   "someid",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}

	token := c.PublishUplink(dataUp)
	waitForOK(token, a)
}

func TestPublishUplinkFields(t *testing.T) {
	a := New(t)
	ctx := GetLogger(t, "Test")
	c := NewClient(ctx, "test", "", "", fmt.Sprintf("tcp://%s:1883", host))

	c.Connect()
	defer c.Disconnect()

	waitChan := make(chan bool, 1)
	expected := 8
	subToken := c.(*DefaultClient).mqtt.Subscribe("fields-app/devices/fields-dev/up/#", QoS, func(_ MQTT.Client, msg MQTT.Message) {

		switch strings.TrimPrefix(msg.Topic(), "fields-app/devices/fields-dev/up/") {
		case "battery":
			a.So(string(msg.Payload()), ShouldEqual, "90")
		case "sensors":
			a.So(string(msg.Payload()), ShouldContainSubstring, `people":["`)
		case "sensors/color":
			a.So(string(msg.Payload()), ShouldEqual, `"blue"`)
		case "sensors/people":
			a.So(string(msg.Payload()), ShouldEqual, `["bob","alice"]`)
		case "sensors/water":
			a.So(string(msg.Payload()), ShouldEqual, "true")
		case "sensors/analog":
			a.So(string(msg.Payload()), ShouldEqual, `[0,255,500,1000]`)
		case "sensors/history":
			a.So(string(msg.Payload()), ShouldContainSubstring, `today":"`)
		case "sensors/history/today":
			a.So(string(msg.Payload()), ShouldEqual, `"not yet"`)
		case "sensors/history/yesterday":
			a.So(string(msg.Payload()), ShouldEqual, `"absolutely"`)
		case "gps":
			a.So(string(msg.Payload()), ShouldEqual, "[52.3736735,4.886663]")
		default:
			t.Errorf("Should not have received message on topic %s", msg.Topic())
			t.Fail()
		}

		expected--
		if expected == 0 {
			waitChan <- true
		}
	})
	waitForOK(subToken, a)

	fields := map[string]interface{}{
		"battery": 90,
		"sensors": map[string]interface{}{
			"color":  "blue",
			"people": []string{"bob", "alice"},
			"water":  true,
			"analog": []int{0, 255, 500, 1000},
			"history": map[string]interface{}{
				"today":     "not yet",
				"yesterday": "absolutely",
			},
		},
		"gps": []float64{52.3736735, 4.886663},
	}

	pubToken := c.PublishUplinkFields("fields-app", "fields-dev", fields)
	waitForOK(pubToken, a)

	select {
	case <-waitChan:
	case <-time.After(1 * time.Second):
		panic("Did not receive fields")
	}
}

func TestSubscribeDeviceUplink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	subToken := c.SubscribeDeviceUplink("someid", "someid", func(client Client, appID string, devID string, req UplinkMessage) {

	})
	waitForOK(subToken, a)

	unsubToken := c.UnsubscribeDeviceUplink("someid", "someid")
	waitForOK(unsubToken, a)
}

func TestSubscribeAppUplink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	subToken := c.SubscribeAppUplink("someid", func(client Client, appID string, devID string, req UplinkMessage) {

	})
	waitForOK(subToken, a)

	unsubToken := c.UnsubscribeAppUplink("someid")
	waitForOK(unsubToken, a)
}

func TestSubscribeUplink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	subToken := c.SubscribeUplink(func(client Client, appID string, devID string, req UplinkMessage) {

	})
	waitForOK(subToken, a)

	unsubToken := c.UnsubscribeUplink()
	waitForOK(unsubToken, a)
}

func TestPubSubUplink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	waitChan := make(chan bool, 1)

	subToken := c.SubscribeDeviceUplink("app1", "dev1", func(client Client, appID string, devID string, req UplinkMessage) {
		a.So(appID, ShouldResemble, "app1")
		a.So(devID, ShouldResemble, "dev1")

		waitChan <- true
	})
	waitForOK(subToken, a)

	pubToken := c.PublishUplink(UplinkMessage{
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
		AppID:   "app1",
		DevID:   "dev1",
	})
	waitForOK(pubToken, a)

	select {
	case <-waitChan:
	case <-time.After(1 * time.Second):
		panic("Did not receive Uplink")
	}

	unsubToken := c.UnsubscribeDeviceUplink("app1", "dev1")
	waitForOK(unsubToken, a)
}

func TestPubSubAppUplink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	var wg WaitGroup

	wg.Add(2)

	subToken := c.SubscribeAppUplink("app2", func(client Client, appID string, devID string, req UplinkMessage) {
		a.So(appID, ShouldResemble, "app2")
		a.So(req.Payload, ShouldResemble, []byte{0x01, 0x02, 0x03, 0x04})
		wg.Done()
	})
	waitForOK(subToken, a)

	pubToken := c.PublishUplink(UplinkMessage{
		AppID:   "app2",
		DevID:   "dev1",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})
	waitForOK(pubToken, a)
	pubToken = c.PublishUplink(UplinkMessage{
		AppID:   "app2",
		DevID:   "dev2",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})
	waitForOK(pubToken, a)

	a.So(wg.WaitFor(200*time.Millisecond), ShouldBeNil)

	unsubToken := c.UnsubscribeAppUplink("app1")
	waitForOK(unsubToken, a)
}

// Downlink pub/sub

func TestPublishDownlink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	dataDown := DownlinkMessage{
		AppID:   "someid",
		DevID:   "someid",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}

	token := c.PublishDownlink(dataDown)
	waitForOK(token, a)

	a.So(token.Error(), ShouldBeNil)
}

func TestSubscribeDeviceDownlink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	token := c.SubscribeDeviceDownlink("someid", "someid", func(client Client, appID string, devID string, req DownlinkMessage) {

	})
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)

	token = c.UnsubscribeDeviceDownlink("someid", "someid")
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)
}

func TestSubscribeAppDownlink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	token := c.SubscribeAppDownlink("someid", func(client Client, appID string, devID string, req DownlinkMessage) {

	})
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)

	token = c.UnsubscribeAppDownlink("someid")
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)
}

func TestSubscribeDownlink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	token := c.SubscribeDownlink(func(client Client, appID string, devID string, req DownlinkMessage) {

	})
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)

	token = c.UnsubscribeDownlink()
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)
}

func TestPubSubDownlink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	var wg WaitGroup

	wg.Add(1)

	subToken := c.SubscribeDeviceDownlink("app3", "dev3", func(client Client, appID string, devID string, req DownlinkMessage) {
		a.So(appID, ShouldResemble, "app3")
		a.So(devID, ShouldResemble, "dev3")

		wg.Done()
	})
	waitForOK(subToken, a)

	pubToken := c.PublishDownlink(DownlinkMessage{
		AppID:   "app3",
		DevID:   "dev3",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})
	waitForOK(pubToken, a)

	a.So(wg.WaitFor(200*time.Millisecond), ShouldBeNil)

	unsubToken := c.UnsubscribeDeviceDownlink("app3", "dev3")
	waitForOK(unsubToken, a)
}

func TestPubSubAppDownlink(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	var wg WaitGroup

	wg.Add(2)

	subToken := c.SubscribeAppDownlink("app4", func(client Client, appID string, devID string, req DownlinkMessage) {
		a.So(appID, ShouldResemble, "app4")
		a.So(req.Payload, ShouldResemble, []byte{0x01, 0x02, 0x03, 0x04})
		wg.Done()
	})
	waitForOK(subToken, a)

	pubToken := c.PublishDownlink(DownlinkMessage{
		AppID:   "app4",
		DevID:   "dev1",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})
	waitForOK(pubToken, a)
	pubToken = c.PublishDownlink(DownlinkMessage{
		AppID:   "app4",
		DevID:   "dev2",
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})
	waitForOK(pubToken, a)

	a.So(wg.WaitFor(200*time.Millisecond), ShouldBeNil)

	unsubToken := c.UnsubscribeAppDownlink("app3")
	waitForOK(unsubToken, a)
}

// Activations pub/sub

func TestPublishActivations(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	dataActivations := Activation{
		AppID:    "someid",
		DevID:    "someid",
		Metadata: Metadata{DataRate: "SF7BW125"},
	}

	token := c.PublishActivation(dataActivations)
	waitForOK(token, a)

	a.So(token.Error(), ShouldBeNil)
}

func TestSubscribeDeviceActivations(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	token := c.SubscribeDeviceActivations("someid", "someid", func(client Client, appID string, devID string, req Activation) {

	})
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)

	token = c.UnsubscribeDeviceActivations("someid", "someid")
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)
}

func TestSubscribeAppActivations(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	token := c.SubscribeAppActivations("someid", func(client Client, appID string, devID string, req Activation) {

	})
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)

	token = c.UnsubscribeAppActivations("someid")
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)
}

func TestSubscribeActivations(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	token := c.SubscribeActivations(func(client Client, appID string, devID string, req Activation) {

	})
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)

	token = c.UnsubscribeActivations()
	waitForOK(token, a)
	a.So(token.Error(), ShouldBeNil)
}

func TestPubSubActivations(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	var wg WaitGroup

	wg.Add(1)

	subToken := c.SubscribeDeviceActivations("app5", "dev1", func(client Client, appID string, devID string, req Activation) {
		a.So(appID, ShouldResemble, "app5")
		a.So(devID, ShouldResemble, "dev1")

		wg.Done()
	})
	waitForOK(subToken, a)

	pubToken := c.PublishActivation(Activation{
		AppID:    "app5",
		DevID:    "dev1",
		Metadata: Metadata{DataRate: "SF7BW125"},
	})
	waitForOK(pubToken, a)

	a.So(wg.WaitFor(200*time.Millisecond), ShouldBeNil)

	unsubToken := c.UnsubscribeDeviceActivations("app5", "dev1")
	waitForOK(unsubToken, a)
}

func TestPubSubAppActivations(t *testing.T) {
	a := New(t)
	c := NewClient(GetLogger(t, "Test"), "test", "", "", fmt.Sprintf("tcp://%s:1883", host))
	c.Connect()
	defer c.Disconnect()

	var wg WaitGroup

	wg.Add(2)

	subToken := c.SubscribeAppActivations("app6", func(client Client, appID string, devID string, req Activation) {
		a.So(appID, ShouldResemble, "app6")
		a.So(req.Metadata.DataRate, ShouldEqual, "SF7BW125")
		wg.Done()
	})
	waitForOK(subToken, a)

	pubToken := c.PublishActivation(Activation{
		AppID:    "app6",
		DevID:    "dev1",
		Metadata: Metadata{DataRate: "SF7BW125"},
	})
	waitForOK(pubToken, a)
	pubToken = c.PublishActivation(Activation{
		AppID:    "app6",
		DevID:    "dev2",
		Metadata: Metadata{DataRate: "SF7BW125"},
	})
	waitForOK(pubToken, a)

	a.So(wg.WaitFor(200*time.Millisecond), ShouldBeNil)

	unsubToken := c.UnsubscribeAppActivations("app6")
	waitForOK(unsubToken, a)
}

func ExampleNewClient() {
	ctx := log.WithField("Example", "NewClient")
	exampleClient := NewClient(ctx, "ttnctl", "my-app-id", "my-access-key", "staging.thethingsnetwork.org:1883")
	err := exampleClient.Connect()
	if err != nil {
		ctx.WithError(err).Fatal("Could not connect")
	}
}

var exampleClient Client

func ExampleDefaultClient_SubscribeDeviceUplink() {
	token := exampleClient.SubscribeDeviceUplink("my-app-id", "my-dev-id", func(client Client, appID string, devID string, req UplinkMessage) {
		// Do something with the message
	})
	token.Wait()
	if err := token.Error(); err != nil {
		panic(err)
	}
}

func ExampleDefaultClient_PublishDownlink() {
	token := exampleClient.PublishDownlink(DownlinkMessage{
		AppID:   "my-app-id",
		DevID:   "my-dev-id",
		FPort:   1,
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})
	token.Wait()
	if err := token.Error(); err != nil {
		panic(err)
	}
}
