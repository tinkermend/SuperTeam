package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
)

func main() {
	ownerID := uuid.New()
	jsonStr := `{"human_owner_user_ids":["` + ownerID.String() + `"]}`
	fmt.Printf("JSON: %s\n", jsonStr)

	var req struct {
		HumanOwnerUserIDs []uuid.UUID `json:"human_owner_user_ids,omitempty"`
	}

	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Decoded: %#v\n", req.HumanOwnerUserIDs)
	}
}
