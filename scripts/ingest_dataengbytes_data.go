package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type User struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Position string   `json:"position"`
	Bio      string   `json:"bio"`
	Linkedin string   `json:"linkedin"`
	Roles    []string `json:"roles"`
}

type Session struct {
	Title       string   `json:"title"`
	SpeakerIDs  []string `json:"speakerIds"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Abstract    string   `json:"abstract"`
	Cities      []string `json:"cities"`
	Tags        []string `json:"tags"`
	Level       string   `json:"level"`
	Duration    any      `json:"duration"` // can be int or string
}

type Conference struct {
	Location string `json:"location"`
	Address  string `json:"address"`
	Date     string `json:"date"`
}

type Operation struct {
	Op     string    `json:"op"`
	Entity *Entity   `json:"entity,omitempty"`
	From   string    `json:"from,omitempty"`
	To     string    `json:"to,omitempty"`
	Rel    string    `json:"rel,omitempty"`
	Ts     time.Time `json:"ts"`
}

type Entity struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Created    time.Time      `json:"created"`
	Updated    time.Time      `json:"updated"`
	ValidFrom  time.Time      `json:"valid_from"`
}

func slugify(text string) string {
	text = strings.ToLower(text)
	reg := regexp.MustCompile(`[^\w\s-]`)
	text = reg.ReplaceAllString(text, "")
	regSpace := regexp.MustCompile(`[\s_]+`)
	text = regSpace.ReplaceAllString(text, "-")
	return strings.Trim(text, "-")
}

func main() {
	targetDir := "/Users/peterhanssens/consulting/DataEngBytes/dataengbytes.com"
	dataDir := filepath.Join(targetDir, "public/data/2025")

	usersPath := filepath.Join(dataDir, "users.json")
	sessionsPath := filepath.Join(dataDir, "sessions.json")
	conferencePath := filepath.Join(dataDir, "conference.json")

	ontologyDir := filepath.Join(targetDir, ".ontology")
	graphPath := filepath.Join(ontologyDir, "graph.jsonl")
	dbPath := filepath.Join(ontologyDir, "graph.db")

	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		fmt.Printf("Error: graph.jsonl not found at %s\n", graphPath)
		return
	}

	fmt.Printf("Reading data files from %s...\n", dataDir)

	// 1. Read users
	userData, err := os.ReadFile(usersPath)
	if err != nil {
		fmt.Printf("Error reading users: %v\n", err)
		return
	}
	var users []User
	if err := json.Unmarshal(userData, &users); err != nil {
		fmt.Printf("Error parsing users: %v\n", err)
		return
	}

	// 2. Read sessions
	sessionData, err := os.ReadFile(sessionsPath)
	if err != nil {
		fmt.Printf("Error reading sessions: %v\n", err)
		return
	}
	var sessions map[string]Session
	if err := json.Unmarshal(sessionData, &sessions); err != nil {
		fmt.Printf("Error parsing sessions: %v\n", err)
		return
	}

	// 3. Read conferences
	confData, err := os.ReadFile(conferencePath)
	if err != nil {
		fmt.Printf("Error reading conferences: %v\n", err)
		return
	}
	var conferences []Conference
	if err := json.Unmarshal(confData, &conferences); err != nil {
		fmt.Printf("Error parsing conferences: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d users, %d sessions, %d conferences.\n", len(users), len(sessions), len(conferences))

	var newOps []Operation
	createdIDs := make(map[string]bool)

	now := time.Now().UTC()

	// 1. Create Conferences as Plans
	for _, conf := range conferences {
		confID := fmt.Sprintf("Plan:conf-%s-2025", slugify(conf.Location))
		if !createdIDs[confID] {
			newOps = append(newOps, Operation{
				Op: "create",
				Entity: &Entity{
					ID:   confID,
					Type: "Plan",
					Properties: map[string]any{
						"title":    fmt.Sprintf("DataEngBytes %s 2025 Conference", conf.Location),
						"kind":     "eng",
						"status":   "landed",
						"location": conf.Location,
						"address":  conf.Address,
						"date":     conf.Date,
					},
					Created:   now,
					Updated:   now,
					ValidFrom: now,
				},
				Ts: now,
			})
			createdIDs[confID] = true
		}
	}

	// 2. Create Users as Persons
	for _, user := range users {
		userID := fmt.Sprintf("Person:%s", user.ID)
		if !createdIDs[userID] {
			newOps = append(newOps, Operation{
				Op: "create",
				Entity: &Entity{
					ID:   userID,
					Type: "Person",
					Properties: map[string]any{
						"name":        user.Name,
						"role":        user.Position,
						"bio":         user.Bio,
						"linkedin":    user.Linkedin,
						"roles_list":  strings.Join(user.Roles, ", "),
					},
					Created:   now,
					Updated:   now,
					ValidFrom: now,
				},
				Ts: now,
			})
			createdIDs[userID] = true
		}
	}

	// 3. Create Taxonomy Terms for unique tags
	allTags := make(map[string]bool)
	for _, sess := range sessions {
		for _, tag := range sess.Tags {
			allTags[tag] = true
		}
	}

	for tag := range allTags {
		tagID := fmt.Sprintf("Term:%s", slugify(tag))
		if !createdIDs[tagID] {
			newOps = append(newOps, Operation{
				Op: "create",
				Entity: &Entity{
					ID:   tagID,
					Type: "Term",
					Properties: map[string]any{
						"label":      tag,
						"definition": fmt.Sprintf("Taxonomy concept representing %s", tag),
						"status":     "proposed",
					},
					Created:   now,
					Updated:   now,
					ValidFrom: now,
				},
				Ts: now,
			})
			createdIDs[tagID] = true
		}
	}

	// 4. Create Sessions as Beads + Relations
	for sessIDRaw, sess := range sessions {
		sessID := fmt.Sprintf("Bead:%s", sessIDRaw)
		if !createdIDs[sessID] {
			desc := sess.Description
			if desc == "" {
				desc = sess.Abstract
			}
			level := sess.Level
			if level == "" {
				level = "All Levels"
			}
			durationStr := fmt.Sprintf("%v", sess.Duration)

			newOps = append(newOps, Operation{
				Op: "create",
				Entity: &Entity{
					ID:   sessID,
					Type: "Bead",
					Properties: map[string]any{
						"title":       sess.Title,
						"kind":        "research",
						"status":      "done",
						"description": desc,
						"level":       level,
						"duration":    durationStr,
					},
					Created:   now,
					Updated:   now,
					ValidFrom: now,
				},
				Ts: now,
			})
			createdIDs[sessID] = true

			// Relations:
			// - Speaker owns Session
			for _, spkID := range sess.SpeakerIDs {
				personID := fmt.Sprintf("Person:%s", spkID)
				newOps = append(newOps, Operation{
					Op:   "relate",
					From: personID,
					To:   sessID,
					Rel:  "owns",
					Ts:   now,
				})
			}

			// - Session tagged_with tags
			for _, tag := range sess.Tags {
				tagID := fmt.Sprintf("Term:%s", slugify(tag))
				newOps = append(newOps, Operation{
					Op:   "relate",
					From: sessID,
					To:   tagID,
					Rel:  "tagged_with",
					Ts:   now,
				})
			}

			// - Session implements Conference (Plan)
			for _, city := range sess.Cities {
				confID := fmt.Sprintf("Plan:conf-%s-2025", slugify(city))
				newOps = append(newOps, Operation{
					Op:   "relate",
					From: sessID,
					To:   confID,
					Rel:  "implements",
					Ts:   now,
				})
			}
		}
	}

	fmt.Printf("Generated %d new operations. Appending to %s...\n", len(newOps), graphPath)

	// Append to JSONL log file
	f, err := os.OpenFile(graphPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Printf("Error opening graph file for append: %v\n", err)
		return
	}
	defer f.Close()

	for _, op := range newOps {
		data, err := json.Marshal(op)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			fmt.Printf("Error writing operation: %v\n", err)
			return
		}
	}

	fmt.Println("Log file appended successfully.")

	// Trigger SQLite Rebuild by deleting graph.db
	if _, err := os.Stat(dbPath); err == nil {
		fmt.Printf("Removing stale graph.db at %s to trigger full auto-recovery...\n", dbPath)
		if err := os.Remove(dbPath); err != nil {
			fmt.Printf("Error removing database: %v\n", err)
		} else {
			fmt.Println("Stale database file removed.")
		}
	}

	fmt.Println("✓ Golang Ingestion complete!")
}
