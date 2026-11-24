package membership

import "encoding/json"

// Session is a booked hour plus the workout the trainer wrote.
type Session struct {
	Start       string
	End         string
	Status      string
	TrainerName string
	ClientName  string
	Note        string
	Sets        int
	Reps        int
	Load        string
	SoloPlan    []string
}

type timetableJSON struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Status      string `json:"status"`
	TrainerName string `json:"trainer_name"`
	ClientName  string `json:"client_name"`
}

type workoutJSON struct {
	timetableJSON
	Note     string   `json:"note"`
	Sets     int      `json:"sets"`
	Reps     int      `json:"reps"`
	Load     string   `json:"load"`
	SoloPlan []string `json:"solo_plan,omitempty"`
}

// MarshalTimetable returns the hour a caller may see.
// A manager and the fitness center account get the timetable only.
func MarshalTimetable(role Role, session Session) ([]byte, error) {
	base := timetableJSON{
		Start:       session.Start,
		End:         session.End,
		Status:      session.Status,
		TrainerName: session.TrainerName,
		ClientName:  session.ClientName,
	}
	switch role {
	case RoleClient, RoleTrainer:
		return json.Marshal(workoutJSON{
			timetableJSON: base,
			Note:          session.Note,
			Sets:          session.Sets,
			Reps:          session.Reps,
			Load:          session.Load,
			SoloPlan:      session.SoloPlan,
		})
	case RoleManager, RoleCenterAdmin:
		return json.Marshal(base)
	default:
		return nil, forbidden()
	}
}
