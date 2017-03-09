package plugins

import (
    "github.com/Seklfreak/Robyul2/helpers"
    "github.com/bwmarrin/discordgo"
    "github.com/Jeffail/gabs"
    "fmt"
    "net/http"
    "strings"
    "bytes"
    "io"
    "github.com/Seklfreak/Robyul2/logger"
    "time"
)

const (
    gfycatApiBaseUrl  = "https://api.gfycat.com/v1/%s"
    gfycatFriendlyUrl = "https://gfycat.com/%s"
)

type Gfycat struct{}

func (m *Gfycat) Commands() []string {
    return []string{
        "gfycat",
        "gfy",
    }
}

func (m *Gfycat) Init(session *discordgo.Session) {

}

func (m *Gfycat) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) { // [p]gfy [<link>] or attachment
    session.ChannelTyping(msg.ChannelID)

    if len(content) <= 0 && len(msg.Attachments) <= 0 {
        _, err := session.ChannelMessageSend(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
        helpers.Relax(err)
        return
    }

    sourceUrl := content
    if len(msg.Attachments) > 0 {
        sourceUrl = msg.Attachments[0].URL
    }

    accessToken := m.getAccessToken()

    httpClient = &http.Client{}

    postGfycatEndpoint := fmt.Sprintf(gfycatApiBaseUrl, "gfycats")
    postData, err := gabs.ParseJSON([]byte(fmt.Sprintf(
        `{"private": true,
    "fetchUrl": "%s"}`,
        sourceUrl,
    )))
    helpers.Relax(err)
    request, err := http.NewRequest("POST", postGfycatEndpoint, strings.NewReader(postData.String()))
    request.Header.Add("user-agent", helpers.DEFAULT_UA)
    request.Header.Add("content-type", "application/json")
    request.Header.Add("Authorization", accessToken)
    helpers.Relax(err)
    response, err := httpClient.Do(request)
    helpers.Relax(err)
    defer response.Body.Close()
    buf := bytes.NewBuffer(nil)
    _, err = io.Copy(buf, response.Body)
    helpers.Relax(err)
    jsonResult, err := gabs.ParseJSON(buf.Bytes())
    helpers.Relax(err)

    session.ChannelMessageSend(msg.ChannelID, "Your gfycat is processing, this may take a while. :sleeping:")
    session.ChannelTyping(msg.ChannelID)

    if jsonResult.ExistsP("isOk") == false || jsonResult.Path("isOk").Data().(bool) == false {
        _, err := session.ChannelMessageSend(msg.ChannelID, helpers.GetTextF("bot.errors.general", "Gfycat Error"))
        helpers.Relax(err)
        logger.ERROR.L("gfycat", fmt.Sprintf("Gfycat Error: %s", jsonResult.String()))
        return
    }

    gfyName := jsonResult.Path("gfyname").Data().(string)
CheckGfycatStatusLoop:
    for {
        statusGfycatEndpoint := fmt.Sprintf(gfycatApiBaseUrl, fmt.Sprintf("gfycats/fetch/status/%s", gfyName))
        result := helpers.GetJSON(statusGfycatEndpoint)

        switch result.Path("task").Data().(string) {
        case "encoding":
            time.Sleep(5 * time.Second)
            session.ChannelTyping(msg.ChannelID)
            continue CheckGfycatStatusLoop
        case "complete":
            gfyName = result.Path("gfyname").Data().(string)
            break CheckGfycatStatusLoop
        default:
            logger.ERROR.L("gfycat", fmt.Sprintf("Gfycat Status Error: %s", result.String()))
            _, err := session.ChannelMessage(msg.ChannelID, helpers.GetTextF("bot.errors.general", "Gfycat Status Error"))
            helpers.Relax(err)
            return
        }
    }

    gfycatUrl := fmt.Sprintf(gfycatFriendlyUrl, gfyName)

    _, err = session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("<@%s> Your gfycat is done: %s .", msg.Author.ID, gfycatUrl))
    helpers.Relax(err)
}

func (m *Gfycat) getAccessToken() string {
    getTokenEndpoint := fmt.Sprintf(gfycatApiBaseUrl, "oauth/token")
    postData, err := gabs.ParseJSON([]byte(fmt.Sprintf(
        `{"grant_type": "client_credentials",
    "client_id": "%s",
    "client_secret": "%s"}`,
        helpers.GetConfig().Path("gfycat.client_id").Data().(string),
        helpers.GetConfig().Path("gfycat.client_secret").Data().(string),
    )))
    helpers.Relax(err)
    httpClient = &http.Client{}
    request, err := http.NewRequest("POST", getTokenEndpoint, strings.NewReader(postData.String()))
    request.Header.Add("user-agent", helpers.DEFAULT_UA)
    helpers.Relax(err)
    response, err := httpClient.Do(request)
    helpers.Relax(err)
    defer response.Body.Close()
    buf := bytes.NewBuffer(nil)
    _, err = io.Copy(buf, response.Body)
    helpers.Relax(err)
    jsonResult, err := gabs.ParseJSON(buf.Bytes())
    helpers.Relax(err)

    tokenType := jsonResult.Path("token_type").Data().(string)
    accessToken := jsonResult.Path("access_token").Data().(string)

    return strings.Title(tokenType) + " " + accessToken
}