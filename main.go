package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/bytedance/gopkg/util/logger"
	"github.com/google/generative-ai-go/genai"
	"github.com/spf13/viper"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const (
	XXTestChannel = "1182270529974046760"
)

var (
	AppID        string
	GuildID      string
	DiscordToken string
	APIKey       string
)

func Init() {
	viper.SetConfigFile(".env")
	_ = viper.ReadInConfig()

	if err := viper.BindEnv("APP_ID"); err != nil {
		log.Fatal(err)
	}
	AppID = viper.GetString("APP_ID")

	if err := viper.BindEnv("GUILD_ID"); err != nil {
		log.Fatal(err)
	}
	GuildID = viper.GetString("GUILD_ID")

	if err := viper.BindEnv("DISCORD_TOKEN"); err != nil {
		log.Fatal(err)
	}
	DiscordToken = viper.GetString("DISCORD_TOKEN")

	if err := viper.BindEnv("API_KEY"); err != nil {
		log.Fatal(err)
	}
	APIKey = viper.GetString("API_KEY")
}

func main() {
	Init()

	dg, err := discordgo.New("Bot " + DiscordToken)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	logger.Infof("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	_ = dg.Close()
}

var cs *genai.ChatSession

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	if len(m.Attachments) > 0 {
		// Validate the image type
		if !validateImageType(m.Attachments[0].Filename) {
			s.ChannelMessageSend(m.ChannelID, "Invalid image type. Please upload a jpeg image.")
			return
		}

		imgData, err := downloadImage(m.Attachments[0].URL)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "An error occurred processing the image.")
			return
		}

		ctx := context.Background()
		client, err := genai.NewClient(ctx, option.WithAPIKey(APIKey))
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		// Model name depends on your task
		model := client.GenerativeModel("gemini-pro-vision")
		message, err := s.ChannelMessageSendReply(m.ChannelID, "Thinking...", m.Reference())
		if err != nil {
			log.Fatal(err)
		}

		img := genai.ImageData("jpeg", imgData)
		var prompt genai.Text
		if len(m.Content) > 0 {
			prompt = genai.Text(m.Content)
		} else {
			prompt = "describe this photo"
		}
		iter := model.GenerateContentStream(ctx, img, prompt)
		var completion string
		for {
			resp, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("completion: %+v", resp.Candidates[0])
			completion += string(resp.Candidates[0].Content.Parts[0].(genai.Text))
			s.ChannelMessageEdit(m.ChannelID, message.ID, completion)
		}

	} else if m.Content != "" {
		ctx := context.Background()
		client, err := genai.NewClient(ctx, option.WithAPIKey(APIKey))
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		message, err := s.ChannelMessageSendReply(m.ChannelID, "Thinking...", m.Reference())
		if err != nil {
			log.Fatal(err)
		}

		// For text-only input, use the gemini-pro model
		model := client.GenerativeModel("gemini-pro")

		if cs == nil {
			cs = model.StartChat()
		}

		iter := cs.SendMessageStream(ctx, genai.Text(m.Content))
		var completion string
		for {
			resp, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("completion: %+v", resp.Candidates[0])
			completion += string(resp.Candidates[0].Content.Parts[0].(genai.Text))
			s.ChannelMessageEdit(m.ChannelID, message.ID, completion)
		}
	}
}

func validateImageType(filename string) bool {
	// Check the suffix of filename
	suffixes := []string{".jpg", ".jpeg", ".png", ".webp"} // Add more as necessary
	for _, suffix := range suffixes {
		if strings.HasSuffix(filename, suffix) {
			return true
		}
	}
	return false
}

func downloadImage(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return bufio.NewReader(resp.Body).ReadBytes('\n')
}
