package biasgame

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"sort"
	"strings"
	"time"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/models"
	"github.com/bwmarrin/discordgo"
	"github.com/globalsign/mgo/bson"
	"github.com/nfnt/resize"
	"github.com/sethgrid/pester"
)

const (
	CHECKMARK_EMOJI    = "✅"
	X_EMOJI            = "❌"
	QUESTIONMARK_EMOJI = "❓"
	NAV_NUMBERS_EMOJI  = "🔢"

	MAX_IMAGE_SIZE = 2000 // 2000x2000px
	MIN_IMAGE_SIZE = 150  // 150x150px
)

var imageSuggestionChannlId string
var imageSuggestionChannel *discordgo.Channel
var suggestionQueue []*models.BiasGameSuggestionEntry
var suggestionEmbedMessageId string // id of the embed message where suggestions are accepted/denied
var exampleRoundPicId string
var suggestionQueueCountMessageId string
var quoteReplacer = strings.NewReplacer("“", "\"", "”", "\"", "‘", "'", "’", "'")
var predefinedDenyMessages map[int]string

func initSuggestionChannel() {
	var err error

	predefinedDenyMessages = map[int]string{
		1: "The face is too small, please zoom in more.",
		2: "Please only submit pictures in color.",
		3: "The face is too hard to see.",
		4: "The picture quality is too low.",
		5: "Bad cropping.",
	}

	imageSuggestionChannlId = helpers.GetConfig().Path("biasgame.suggestion_channel_id").Data().(string)
	imageSuggestionChannel, err = helpers.GetChannel(imageSuggestionChannlId)
	helpers.Relax(err)

	// when the bot starts, delete any past bot messages from the suggestion channel and make the embed
	var messagesToDelete []string
	messagesInChannel, _ := cache.GetSession().ChannelMessages(imageSuggestionChannlId, 100, "", "", "")
	for _, msg := range messagesInChannel {
		messagesToDelete = append(messagesToDelete, msg.ID)
	}

	cache.GetSession().ChannelMessagesBulkDelete(imageSuggestionChannlId, messagesToDelete)

	// make a message on how to edit suggestions
	helpMessage := "```Editable Fields: name, group, gender, notes\n" +
		"Command: " + helpers.GetPrefixForServer(imageSuggestionChannel.GuildID) + "biasgame-edit {field} new field value...\n\n" +
		"\n1. " + predefinedDenyMessages[1] +
		"\n2. " + predefinedDenyMessages[2] +
		"\n3. " + predefinedDenyMessages[3] +
		"\n4. " + predefinedDenyMessages[4] +
		"\n5. " + predefinedDenyMessages[5] + "```"

	helpers.SendMessage(imageSuggestionChannlId, helpMessage)

	// load unresolved suggestions and create the first embed
	loadUnresolvedSuggestions()
	updateSuggestionQueueCount()
	updateCurrentSuggestionEmbed()
}

// processImageSuggestion
func processImageSuggestion(msg *discordgo.Message, msgContent string) {
	defer helpers.Recover()

	channel, err := helpers.GetChannel(msg.ChannelID)
	helpers.Relax(err)

	// replace odd double and single quotes with real ones
	msgContent = quoteReplacer.Replace(msgContent)

	suggestionArgs, err := helpers.ToArgv(msgContent)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}
	suggestionArgs = suggestionArgs[1:]

	var suggestedImageUrl string

	// validate suggestion arg amount.
	if len(msg.Attachments) == 1 {
		if len(suggestionArgs) != 3 {
			helpers.SendMessage(msg.ChannelID, helpers.GetTextF("plugins.biasgame.suggestion.invalid-suggestion",
				helpers.GetPrefixForServer(channel.GuildID), helpers.GetPrefixForServer(channel.GuildID)))
			return
		}
		suggestedImageUrl = msg.Attachments[0].URL
	} else {
		if len(suggestionArgs) != 4 {
			helpers.SendMessage(msg.ChannelID, helpers.GetTextF("plugins.biasgame.suggestion.invalid-suggestion",
				helpers.GetPrefixForServer(channel.GuildID), helpers.GetPrefixForServer(channel.GuildID)))
			return
		}
		suggestedImageUrl = suggestionArgs[3]
	}

	// set gender to lowercase and check if its valid
	suggestionArgs[0] = strings.ToLower(suggestionArgs[0])
	if suggestionArgs[0] != "girl" && suggestionArgs[0] != "boy" {
		helpers.SendMessage(msg.ChannelID, helpers.GetTextF("plugins.biasgame.suggestion.invalid-suggestion",
			helpers.GetPrefixForServer(channel.GuildID), helpers.GetPrefixForServer(channel.GuildID)))
		return
	}

	// confirm user can upload pictures
	if helpers.UseruploadsIsDisabled(msg.Author.ID) {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.errors.useruploads-disabled"))
		return
	}

	// validate url image
	resp, err := pester.Get(suggestedImageUrl)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.invalid-url"))
		return
	}
	defer resp.Body.Close()

	// make sure image is png or jpeg
	if resp.Header.Get("Content-type") != "image/png" && resp.Header.Get("Content-type") != "image/jpeg" {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.not-png-or-jpeg"))
		return
	}

	// attempt to decode the image, if we can't there may be something wrong with the image submitted
	suggestedImage, _, errr := image.Decode(resp.Body)
	if errr != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.invalid-url"))
		return
	}

	// Check height and width are equal
	if suggestedImage.Bounds().Dy() != suggestedImage.Bounds().Dx() {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.image-not-square"))
		return
	}

	// Validate size of image
	if suggestedImage.Bounds().Dy() > MAX_IMAGE_SIZE || suggestedImage.Bounds().Dy() < MIN_IMAGE_SIZE {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.invalid-image-size"))
		return
	}

	// validate group and idol name have no double quotes or underscores
	if strings.ContainsAny(suggestionArgs[1]+suggestionArgs[2], "\"_") {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.invalid-group-or-idol"))
		return
	}

	sugImgHashString, err := helpers.GetImageHashString(suggestedImage)
	helpers.Relax(err)

	// compare the given image to all images currently available in the game
	for _, bias := range getAllBiases() {
		for _, curBImage := range bias.BiasImages {
			compareVal, err := helpers.ImageHashStringComparison(sugImgHashString, curBImage.HashString)
			if err != nil {
				bgLog().Errorf("Comparison error: %s", err.Error())
				continue
			}

			// if the difference is 1 or less let the user know the image already exists
			if compareVal <= 1 {
				helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.suggested-image-exists"))
				return
			}
		}
	}

	// compare the given image to all images currently in the suggestion queue
	for _, suggestion := range suggestionQueue {
		compareVal, err := helpers.ImageHashStringComparison(sugImgHashString, suggestion.ImageHashString)
		if err != nil {
			bgLog().Errorf("Comparison error: %s", err.Error())
			continue
		}

		// if the difference is 1 or less let the user know the image already exists
		if compareVal <= 1 {
			helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.suggestion.image-is-suggested"))
			return
		}
	}

	// must resize image when suggestion is made. the same file that
	//   is created by the suggested will be used by the game if its accepted
	suggestedImage = resize.Resize(0, IMAGE_RESIZE_HEIGHT, suggestedImage, resize.Lanczos3)

	// upload file
	buf := new(bytes.Buffer)
	err = png.Encode(buf, suggestedImage)
	helpers.Relax(err)
	objectName, err := helpers.AddFile("", buf.Bytes(), helpers.AddFileMetadata{
		Filename:           suggestedImageUrl,
		ChannelID:          msg.ChannelID,
		UserID:             msg.Author.ID,
		AdditionalMetadata: nil,
	}, "biasgame", false)
	helpers.Relax(err)

	// send ty message
	helpers.SendMessage(msg.ChannelID, helpers.GetTextF("plugins.biasgame.suggestion.thanks-for-suggestion", msg.Author.Mention()))

	// create suggetion
	suggestion := &models.BiasGameSuggestionEntry{
		UserID:          msg.Author.ID,
		ChannelID:       msg.ChannelID,
		Gender:          suggestionArgs[0],
		GrouopName:      suggestionArgs[1],
		Name:            suggestionArgs[2],
		ImageURL:        suggestedImageUrl,
		ImageHashString: sugImgHashString,
		GroupMatch:      false,
		IdolMatch:       false,
		ObjectName:      objectName,
	}
	checkIdolAndGroupExist(suggestion)

	// save suggetion to database and memory
	suggestionQueue = append(suggestionQueue, suggestion)
	helpers.MDbInsert(models.BiasGameSuggestionsTable, suggestion)
	updateSuggestionQueueCount()

	if len(suggestionQueue) == 1 || len(suggestionQueue) == 0 {

		updateCurrentSuggestionEmbed()

		// make a message and delete it immediatly. just to show that a new suggestion has come in
		msg, err := helpers.SendMessage(imageSuggestionChannlId, "New Suggestion Ping")
		helpers.Relax(err)
		go helpers.DeleteMessageWithDelay(msg[0], time.Second*2)
	}

}

// CheckSuggestionReaction will check if the reaction was added to a suggestion message
func CheckSuggestionReaction(reaction *discordgo.MessageReactionAdd) {
	var userResponseMessage string

	// check if the reaction added was valid
	if CHECKMARK_EMOJI != reaction.Emoji.Name && X_EMOJI != reaction.Emoji.Name && NAV_NUMBERS_EMOJI != reaction.Emoji.Name {
		return
	}

	// check if the reaction was added to the suggestion embed message
	if reaction.MessageID == suggestionEmbedMessageId {
		if len(suggestionQueue) == 0 {
			return
		}

		cs := suggestionQueue[0]

		// update current page based on direction
		if CHECKMARK_EMOJI == reaction.Emoji.Name {

			// send processing image message
			msg, err := helpers.SendMessage(imageSuggestionChannlId, "Uploading image...")
			if err == nil {
				defer cache.GetSession().ChannelMessageDelete(imageSuggestionChannlId, msg[0].ID)
			}

			addSuggestionToGame(cs)

			// set image accepted image
			userResponseMessage = fmt.Sprintf("**Bias Game Suggestion Approved** <:blobthumbsup:317043177028714497>\nIdol: %s %s\nImage: <%s>", cs.GrouopName, cs.Name, cs.ImageURL)
			cs.Status = "approved"

		} else if X_EMOJI == reaction.Emoji.Name || NAV_NUMBERS_EMOJI == reaction.Emoji.Name {

			// if predefined deny reason
			if NAV_NUMBERS_EMOJI == reaction.Emoji.Name {

				// get the default denial reason
				if reasonNumber, err := getSuggestionDenialInput(imageSuggestionChannlId); err == nil && predefinedDenyMessages[reasonNumber] != "" {
					cs.Notes = predefinedDenyMessages[reasonNumber]
				} else {
					return
				}
			}

			// confirm a note is set before denying a suggestion
			if cs.Notes == "" {
				// remove the x reaction just added
				cache.GetSession().MessageReactionRemove(reaction.ChannelID, reaction.MessageID, reaction.Emoji.Name, reaction.UserID)

				// alert user a note is needed and delete message after delay
				msgs, err := helpers.SendMessage(imageSuggestionChannlId, "A note must be set before denying a suggestion. Please use: `"+helpers.GetPrefixForServer(imageSuggestionChannel.GuildID)+"edit notes {reason for denial...}`")
				helpers.Relax(err)
				helpers.DeleteMessageWithDelay(msgs[0], time.Second*15)
				return
			}

			// image was denied
			userResponseMessage = fmt.Sprintf("**Bias Game Suggestion Denied** <:notlikeblob:349342777978519562>\nIdol: %s %s\nImage: <%s>", cs.GrouopName, cs.Name, cs.ImageURL)
			cs.Status = "denied"

			// remove file from objectstorage
			//  important note: only delete if the image was denied. when an image
			//                  is accepted the same object storage file is used for the game
			go helpers.DeleteFile(cs.ObjectName)
		}

		// update db record
		cs.ProcessedByUserId = reaction.UserID
		cs.LastModifiedOn = time.Now()
		go helpers.MDbUpsertID(models.BiasGameSuggestionsTable, cs.ID, cs)

		// send a message to the user who suggested the image
		dmChannel, err := cache.GetSession().UserChannelCreate(cs.UserID)
		if err == nil {
			// set notes if there are any
			if cs.Notes != "" {
				userResponseMessage += "\nNotes: " + cs.Notes
			}
			go helpers.SendMessage(dmChannel.ID, userResponseMessage)
		}

		// delete first suggestion and process queue again
		suggestionQueue = suggestionQueue[1:]
		go func() {
			defer helpers.Recover()
			updateCurrentSuggestionEmbed()
		}()
	}

	return
}

// UpdateSuggestionDetails
func UpdateSuggestionDetails(msg *discordgo.Message, fieldToUpdate string, value string) {
	if msg.ChannelID != imageSuggestionChannlId {
		return
	}

	if len(suggestionQueue) == 0 {
		return
	}

	go helpers.DeleteMessageWithDelay(msg, time.Second)

	cs := suggestionQueue[0]
	fieldToUpdate = strings.ToLower(fieldToUpdate)

	switch fieldToUpdate {
	case "name":
		cs.Name = value
	case "group":
		cs.GrouopName = value
	case "gender":
		cs.Gender = value
	case "notes":
		cs.Notes = value
	default:
		return
	}

	// save changes and update embed message
	helpers.MDbUpsertID(models.BiasGameSuggestionsTable, cs.ID, cs)
	updateCurrentSuggestionEmbed()
}

// updateCurrentSuggestionEmbed will re-render the embed message with the current suggestion if one exists
func updateCurrentSuggestionEmbed() {
	var embed *discordgo.MessageEmbed
	var msgSend *discordgo.MessageSend
	var cs *models.BiasGameSuggestionEntry

	if exampleRoundPicId != "" {
		go cache.GetSession().ChannelMessageDelete(imageSuggestionChannlId, exampleRoundPicId)
	}

	if len(suggestionQueue) == 0 {

		embed = &discordgo.MessageEmbed{
			Color: 0x0FADED, // blueish
			Author: &discordgo.MessageEmbedAuthor{
				Name: "No suggestions in queue",
			},
		}

		msgSend = &discordgo.MessageSend{Embed: embed}

	} else {
		// current suggestion
		cs = suggestionQueue[0]
		checkIdolAndGroupExist(cs)

		imgBytes, err := helpers.RetrieveFile(cs.ObjectName)
		helpers.Relax(err)

		suggestedImage, _, err := helpers.DecodeImageBytes(imgBytes)
		helpers.Relax(err)

		buf := new(bytes.Buffer)
		encoder := new(png.Encoder)
		encoder.CompressionLevel = -2 // -2 compression is best speed, -3 is best compression but end result isn't worth the slower encoding
		encoder.Encode(buf, makeVSImage(suggestedImage, suggestedImage))
		myReader := bytes.NewReader(buf.Bytes())

		// get info of user who suggested image
		suggestedByText := "*No User Info Found*"
		suggestedBy, err := cache.GetSession().User(cs.UserID)
		if err == nil {
			suggestedByText = fmt.Sprintf("%s#%s \n(%s)", suggestedBy.Username, suggestedBy.Discriminator, suggestedBy.ID)
		}

		// get guild and channel info it was suggested from
		suggestedFromText := "*No Guild Info Found*"
		suggestedFromCh, err := cache.GetSession().Channel(cs.ChannelID)
		if err == nil {

			suggestedFrom, err := cache.GetSession().Guild(suggestedFromCh.GuildID)
			if err == nil {
				suggestedFromText = fmt.Sprintf("G: %s \nC: #%s", suggestedFrom.Name, suggestedFromCh.Name)
			}
		}

		// if the group name and idol name were matched show a checkmark, otherwise show a question mark
		groupNameDisplay := "Group Name"
		if cs.GroupMatch == true {
			groupNameDisplay += " " + CHECKMARK_EMOJI
		} else {
			groupNameDisplay += " " + QUESTIONMARK_EMOJI
		}
		idolNameDisplay := "Idol Name"
		if cs.IdolMatch == true {
			idolNameDisplay += " " + CHECKMARK_EMOJI
		} else {
			idolNameDisplay += " " + QUESTIONMARK_EMOJI
		}

		// check if notes are set, if not then display no notes entered.
		//  discord embeds can't have empty field values
		notesValue := cs.Notes
		if notesValue == "" {
			notesValue = "*No notes entered*"
		}

		embed = &discordgo.MessageEmbed{
			Color: 0x0FADED, // blueish
			Image: &discordgo.MessageEmbedImage{
				URL: "attachment://example_round.png",
			},
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   idolNameDisplay,
					Value:  cs.Name,
					Inline: true,
				},
				{
					Name:   groupNameDisplay,
					Value:  cs.GrouopName,
					Inline: true,
				},
				{
					Name:   "Gender",
					Value:  cs.Gender,
					Inline: true,
				},
				{
					Name:   "Suggested By",
					Value:  suggestedByText,
					Inline: true,
				},
				{
					Name:   "Suggested From",
					Value:  suggestedFromText,
					Inline: true,
				},
				{
					Name:   "Timestamp",
					Value:  cs.ID.Time().Format("Jan 2, 2006 3:04pm (MST)"),
					Inline: true,
				},
				{
					Name:   "Notes",
					Value:  notesValue,
					Inline: true,
				},
				{
					Name:   "Image URL",
					Value:  cs.ImageURL,
					Inline: true,
				},
			},
		}

		msgSend = &discordgo.MessageSend{
			Files: []*discordgo.File{{
				Name:   "example_round.png",
				Reader: myReader,
			}},
			Embed: embed,
		}

	}

	// delete old embed message
	cache.GetSession().ChannelMessageDelete(imageSuggestionChannlId, suggestionEmbedMessageId)

	// delete any other messages in the suggestions channel
	clearSuggestionsChannel()

	// send new embed message
	embedMsg, err := cache.GetSession().ChannelMessageSendComplex(imageSuggestionChannlId, msgSend)
	helpers.Relax(err)
	suggestionEmbedMessageId = embedMsg.ID

	updateSuggestionQueueCount()
	// delete any reactions on message and then reset them if there's another suggestion in queue
	cache.GetSession().MessageReactionsRemoveAll(imageSuggestionChannlId, embedMsg.ID)
	if len(suggestionQueue) > 0 {

		// compare the given image to all images currently available in the game
		sendSimilarImages(embedMsg, cs.ImageHashString)

		cache.GetSession().MessageReactionAdd(imageSuggestionChannlId, embedMsg.ID, CHECKMARK_EMOJI)
		cache.GetSession().MessageReactionAdd(imageSuggestionChannlId, embedMsg.ID, X_EMOJI)
		cache.GetSession().MessageReactionAdd(imageSuggestionChannlId, embedMsg.ID, NAV_NUMBERS_EMOJI)
	}
}

func updateSuggestionQueueCount() {
	// update suggestion count message
	if suggestionQueueCountMessageId == "" {
		msg, err := cache.GetSession().ChannelMessageSend(imageSuggestionChannlId, fmt.Sprintf("Suggestions in queue: %d", len(suggestionQueue)))
		if err == nil {
			suggestionQueueCountMessageId = msg.ID
		}
	} else {
		cache.GetSession().ChannelMessageEdit(imageSuggestionChannlId, suggestionQueueCountMessageId, fmt.Sprintf("Suggestions in queue: %d", len(suggestionQueue)))
	}
}

// loadUnresolvedSuggestions
func loadUnresolvedSuggestions() {
	queryParams := bson.M{}

	queryParams["status"] = ""

	helpers.MDbIter(helpers.MdbCollection(models.BiasGameSuggestionsTable).Find(queryParams)).All(&suggestionQueue)
}

// does a loose comparison of the suggested idols and idols already in the game.
func checkIdolAndGroupExist(sug *models.BiasGameSuggestionEntry) {

	groupMatched, _, matchingBias := getMatchingIdolAndGroup(sug.GrouopName, sug.Name)

	// if a matching idol was found then set the suggested name and group to match
	if matchingBias != nil {
		sug.GrouopName = matchingBias.GroupName
		sug.GroupMatch = true
		sug.Name = matchingBias.BiasName
		sug.IdolMatch = true

	} else if groupMatched {
		// if the group matched, get the group name
		if exist, realGroupName := getMatchingGroup(sug.GrouopName); exist {
			sug.GrouopName = realGroupName
			sug.GroupMatch = true
		}
	}
}

// sendSimilarImages will check for images that are similar to the given images
//  and send them back in a paged embe
func sendSimilarImages(msg *discordgo.Message, sugImgHashString string) {
	matchingImages := make(map[int][]biasImage, 0)
	var compareValues []int

	// compare the given image to all images currently available in the game
	for _, bias := range getAllBiases() {
		for _, curBImage := range bias.BiasImages {
			compareVal, err := helpers.ImageHashStringComparison(sugImgHashString, curBImage.HashString)
			if err != nil {
				bgLog().Errorf("Comparison error: %s", err.Error())
				continue
			}

			if compareVal <= 3 {
				compareValues = append(compareValues, compareVal)
				matchingImages[compareVal] = append(matchingImages[compareVal], curBImage)
			}
		}
	}

	// sort the images by the best match first
	sortedMatchingImages := make([]biasImage, 0)
	sort.Ints(compareValues)
	for _, val := range compareValues {
		sortedMatchingImages = append(sortedMatchingImages, matchingImages[val]...)
		delete(matchingImages, val)
	}

	if len(sortedMatchingImages) > 0 {
		sendPagedEmbedOfImages(msg, sortedMatchingImages, false, "Possible Matching Images", fmt.Sprintf("Images Found: %d", len(sortedMatchingImages)))
	}
}

// clearSuggestionsChannel delete messages in the suggestions channel
//  that are NOT part of the initial setup or the suggestions embed itself
func clearSuggestionsChannel() {

	// if a suggestion embed has not been set then do nothing
	if suggestionEmbedMessageId == "" {
		return
	}

	// get newer messages
	messagesArray, err := cache.GetSession().ChannelMessages(imageSuggestionChannlId, 100, "", suggestionEmbedMessageId, "")
	helpers.Relax(err)

	for _, msg := range messagesArray {
		cache.GetSession().ChannelMessageDelete(imageSuggestionChannlId, msg.ID)
	}
}
