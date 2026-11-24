package membership

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSession() Session {
	return Session{
		Start:       "07:00",
		End:         "08:00",
		Status:      "confirmed",
		TrainerName: "Alex Rivera",
		ClientName:  "Maya Chen",
		Note:        "hips felt tight",
		Sets:        3,
		Reps:        8,
		Load:        "40kg",
		SoloPlan:    []string{"squat"},
	}
}

func TestTimetableOmitsWorkoutForStaff(t *testing.T) {
	for _, role := range []Role{RoleManager, RoleCenterAdmin} {
		t.Run(string(role), func(t *testing.T) {
			raw, err := MarshalTimetable(role, sampleSession())
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(raw, &body))
			assert.Equal(t, "07:00", body["start"])
			assert.Equal(t, "Alex Rivera", body["trainer_name"])
			assert.Equal(t, "Maya Chen", body["client_name"])
			for _, key := range []string{"note", "sets", "reps", "load", "solo_plan"} {
				_, present := body[key]
				assert.False(t, present, key)
			}
		})
	}
}

func TestTimetableKeepsWorkoutForClientAndTrainer(t *testing.T) {
	for _, role := range []Role{RoleClient, RoleTrainer} {
		t.Run(string(role), func(t *testing.T) {
			raw, err := MarshalTimetable(role, sampleSession())
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(raw, &body))
			assert.Equal(t, "hips felt tight", body["note"])
			assert.Equal(t, float64(3), body["sets"])
			assert.Equal(t, float64(8), body["reps"])
			assert.Equal(t, "40kg", body["load"])
			assert.NotEmpty(t, body["solo_plan"])
		})
	}
}
