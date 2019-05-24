package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	chanHistBase = "https://slack.com/api/channels.history?token="
	chanListBase = "https://slack.com/api/channels.list?token="
	token        = "your slack token here" //DONT COMMIT ME!
	count        = 100                     //max is 1000, but use less to make sure loop is still working
	pathEnd      = "&pretty=1"
	first        = "1549587600" //Feb 8th 2019, the day we got slack
)

type slackHistoryResp struct {
	Ok       bool          `json:"ok"`
	Messages []interface{} `json:"messages"`
	HasMore  bool          `json:"has_more"`
}

type slackMsg struct {
	HumanTimestamp string `json:"hst"`
	Timestamp      string `json:"ts"`
	Text           string `json:"text"`
}

type channelInfo struct {
	Name string
	ID   string
}

func main() {

	//get all public slack channels and IDs
	channels, err := getChannels()
	if err != nil {
		log.Fatalf("error getting channels: %s\n", err.Error())
	}

	//create parent directory
	historyDir := fmt.Sprintf("slack_history_%d", time.Now().Unix())
	err = os.Mkdir(historyDir, 0777)
	if err != nil {
		log.Fatalf("error making directory: %s", err.Error())
	}

	//get the history for each channel and write it to a JSON file
	for _, channel := range channels {
		msgs, err := getChannelMessageHistory(channel.ID)
		if err != nil {
			log.Fatalf("failed getting %s channel history: %s", channel, err.Error())
		}

		b, err := json.Marshal(msgs)
		if err != nil {
			log.Fatalf("error marshaling msgs array: %s", err.Error())
		}

		channelPath := path.Join(historyDir, fmt.Sprintf("%s.json", channel.Name))

		//write the JSON to a file named after the channel
		err = ioutil.WriteFile(channelPath, b, 0777)
		if err != nil {
			log.Fatalf("error writing to file: %s", err.Error())
		}
	}
}

func getChannels() ([]channelInfo, error) {
	var channels []channelInfo
	path := fmt.Sprintf("%s%s", chanListBase, token)

	//make the API call
	resp, err := http.Get(path)
	if err != nil {
		return nil, fmt.Errorf("error geting URL: %v\n", err)
	}

	//read response
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v\n", err)
	}

	var responseMap map[string]interface{}

	//unmarshal into map
	err = json.Unmarshal(data, &responseMap)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %s\n", err.Error())
	}

	//get channels array
	channelArray, ok := responseMap["channels"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("error getting channel array from response map")
	}

	//get the name and ID from each channel
	for _, chanInterface := range channelArray {
		channel, ok := chanInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("error asserting channel interface")
		}

		name, ok := channel["name"].(string)
		if !ok {
			return nil, fmt.Errorf("error getting channel name")
		}

		chanId, ok := channel["id"].(string)
		if !ok {
			return nil, fmt.Errorf("error getting channel id")
		}

		newChan := channelInfo{
			Name: name,
			ID:   chanId,
		}

		//add new channel to channels array
		channels = append(channels, newChan)
	}

	return channels, nil
}

func getChannelMessageHistory(channelId string) ([]interface{}, error) {

	start := first
	more := true

	msgs := make([]interface{}, 0)

	//keep requesting more messages while there are more available
	for more {
		//build the url
		path := fmt.Sprintf("%s%s&channel=%s&count=%d&oldest=%s%s", chanHistBase, token, channelId, count, start, pathEnd)

		//make the API call
		resp, err := http.Get(path)
		if err != nil {
			return nil, fmt.Errorf("error getting URL: %v\n", err)
		}

		//read response
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading respons body: %v\n", err)
		}

		//unmarshal the message JSON to get the inner message array
		//leave the message array []interface{} as we don't know all the fields
		//inside and will be marshaling it back to JSON anyway
		var slackHist slackHistoryResp
		err = json.Unmarshal(data, &slackHist)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling API response: %v\n", err)
		}

		//if there are no messages in channel, return
		if len(slackHist.Messages) < 1 {
			return nil, fmt.Errorf("channel has no message history")
		}

		//since the first element in the array is the most recent, iterate though
		//it backwards in chronological order
		for i := len(slackHist.Messages) - 1; i >= 0; i-- {
			//add message to channel message array
			msgs = append(msgs, slackHist.Messages[i])
		}

		//get the timestamp on the first msg, this is the latest msg in the array
		//and will be the lower time limit on our next GET
		latestMsg, ok := slackHist.Messages[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("couldn't get timestamp of latest message")
		}
		start = latestMsg["ts"].(string)

		//the response has a convenient field telling us there is more!
		if !slackHist.HasMore {
			break
		}
	}

	return msgs, nil
}
