package streamerbot

import "stream-guy/internal/render"

type ChatMessageData struct {
	Message struct {
		Message string         `json:"message"`
		Role    int            `json:"role"`
		Emotes  []render.Emote `json:"emotes"`
	} `json:"message"`

	User struct {
		Badges      []BadgeData `json:"badges"`
		ID          string      `json:"id"`
		Login       string      `json:"login"`
		DisplayName string      `json:"name"`
		Color       string      `json:"color"`
	} `json:"user"`
}

type BadgeData struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

func ConvertBadges(badges []BadgeData) []render.Badge {
	result := make([]render.Badge, len(badges))
	for i, b := range badges {
		result[i] = render.Badge{Name: b.Name, ImageURL: b.ImageURL}
	}
	return result
}

type RewardRedemptionData struct {
	Username string `json:"user_name"`
	Reward   struct {
		Title     string `json:"title"`
		UserInput string `json:"user_input"`
	} `json:"reward"`
}
