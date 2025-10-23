package room

type DrawingObject struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Data   map[string]interface{} `json:"data"`
	UserID string                 `json:"userId"`
	ZIndex int                    `json:"zIndex"`
}
