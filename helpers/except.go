// Except.go: Contains functions to make handling panics less PITA

package helpers

import (
    "fmt"
    "github.com/Seklfreak/Robyul2/cache"
    "github.com/bwmarrin/discordgo"
    "github.com/getsentry/raven-go"
    "reflect"
    "runtime"
    "strconv"
)

// RecoverDiscord recover()s and sends a message to discord
func RecoverDiscord(msg *discordgo.Message) {
    err := recover()
    if err != nil {
        SendError(msg, err)
    }
}

// Recover recover()s and prints the error to console
func Recover() {
    err := recover()
    if err != nil {
        fmt.Printf("%#v\n", err)

        //raven.SetUserContext(&raven.User{})
        raven.CaptureError(fmt.Errorf("%#v", err), map[string]string{})
    }
}

// SoftRelax is a softer form of Relax()
// Calls a callback instead of panicking
func SoftRelax(err error, cb Callback) {
    if err != nil {
        cb()
    }
}

// Relax is a helper to reduce if-checks if panicking is allowed
// If $err is nil this is a no-op. Panics otherwise.
func Relax(err error) {
    if err != nil {
        if DEBUG_MODE == true {
            if err, ok := err.(*discordgo.RESTError); ok && err != nil && err.Message != nil {
                fmt.Println(err.Message.Code, err.Message.Message)
            } else {
                fmt.Println(err)
            }
        }
        panic(err)
    }
}

// RelaxAssertEqual panics if a is not b
func RelaxAssertEqual(a interface{}, b interface{}, err error) {
    if !reflect.DeepEqual(a, b) {
        Relax(err)
    }
}

// RelaxAssertUnequal panics if a is b
func RelaxAssertUnequal(a interface{}, b interface{}, err error) {
    if reflect.DeepEqual(a, b) {
        Relax(err)
    }
}

// SendError Takes an error and sends it to discord and sentry.io
func SendError(msg *discordgo.Message, err interface{}) {
    if DEBUG_MODE == true {
        buf := make([]byte, 1<<16)
        stackSize := runtime.Stack(buf, false)

        cache.GetSession().ChannelMessageSend(
            msg.ChannelID,
            "Error <:blobfrowningbig:317028438693117962>\n```\n"+fmt.Sprintf("%#v\n", err)+fmt.Sprintf("%s\n", string(buf[0:stackSize]))+"\n```",
        )
    } else {
        if err, ok := err.(*discordgo.RESTError); ok && err.Message != nil {
            cache.GetSession().ChannelMessageSend(
                msg.ChannelID,
                "Error <:blobfrowningbig:317028438693117962>\nSekl#7397 has been notified.\n```\n"+fmt.Sprintf("%#v", err.Message.Message)+"\n```",
            )
        } else {
            cache.GetSession().ChannelMessageSend(
                msg.ChannelID,
                "Error <:blobfrowningbig:317028438693117962>\nSekl#7397 has been notified.\n```\n"+fmt.Sprintf("%#v", err)+"\n```",
            )
        }
    }

    raven.SetUserContext(&raven.User{
        ID:       msg.ID,
        Username: msg.Author.Username + "#" + msg.Author.Discriminator,
    })

    raven.CaptureError(fmt.Errorf("%#v", err), map[string]string{
        "ChannelID":       msg.ChannelID,
        "Content":         msg.Content,
        "Timestamp":       string(msg.Timestamp),
        "TTS":             strconv.FormatBool(msg.Tts),
        "MentionEveryone": strconv.FormatBool(msg.MentionEveryone),
        "IsBot":           strconv.FormatBool(msg.Author.Bot),
    })
}
