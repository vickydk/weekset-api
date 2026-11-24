package membership

// Access codes. The HTTP adapter maps these to status codes.
const (
	CodeNotFound            = "not_found"
	CodeForbidden           = "forbidden"
	CodeMembershipSuspended = "membership_suspended"
	CodeValidation          = "validation_error"
)

// AccessError is a denied call. Code is what the app branches on.
type AccessError struct {
	Code    string
	Message string
}

func (e *AccessError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// StatusCode is the HTTP status for this denial.
func (e *AccessError) StatusCode() int {
	switch e.Code {
	case CodeNotFound:
		return 404
	case CodeValidation:
		return 400
	default:
		return 403
	}
}

func notFound() error {
	return &AccessError{Code: CodeNotFound, Message: "not_found"}
}

func forbidden() error {
	return &AccessError{Code: CodeForbidden, Message: "forbidden"}
}

func suspended() error {
	return &AccessError{Code: CodeMembershipSuspended, Message: "membership_suspended"}
}

func chooseCenter() error {
	return &AccessError{Code: CodeValidation, Message: "choose a center"}
}

func chooseBranch() error {
	return &AccessError{Code: CodeValidation, Message: "choose a branch"}
}

// Headers are the only place a client or manager names the center or branch.
type Headers struct {
	CenterID string
	BranchID string
}

// BodyClaim is a role, center, or branch sent in a JSON body.
// Those fields never grant access.
type BodyClaim struct {
	Role            string
	FitnessCenterID string
	BranchID        string
}

// Resolve picks the membership for this request.
// A body claim is rejected. A trainer's branch comes from the membership,
// and a branch header is ignored. One active choice is used when the header
// is missing. More than one choice requires the header.
func Resolve(actor Actor, headers Headers, body BodyClaim) (Scope, error) {
	if body.Role != "" || body.FitnessCenterID != "" || body.BranchID != "" {
		return Scope{}, forbidden()
	}
	membership, err := selectMembership(actor, headers.CenterID)
	if err != nil {
		return Scope{}, err
	}
	branchID, err := selectBranch(membership, headers.BranchID)
	if err != nil {
		return Scope{}, err
	}
	return Scope{
		UserID:          actor.UserID,
		FitnessCenterID: membership.FitnessCenterID,
		BranchID:        branchID,
		Role:            membership.Role,
	}, nil
}

// CanReadSchedule allows the caller's own week. A manager or the fitness
// center account gets the timetable only.
func CanReadSchedule(actor Actor, centerID, branchID string) error {
	_, err := scoped(actor, centerID, branchID)
	return err
}

// CanReadSessionLog allows a client to read their own log and a trainer to
// read an assigned client's log. A manager or the fitness center account
// cannot read the note, the sets, the reps, or the load.
func CanReadSessionLog(actor Actor, res Resource) error {
	membership, err := scoped(actor, res.FitnessCenterID, res.BranchID)
	if err != nil {
		return err
	}
	switch membership.Role {
	case RoleClient:
		if res.ClientUserID != actor.UserID {
			return notFound()
		}
		return nil
	case RoleTrainer:
		if res.TrainerUserID != actor.UserID {
			return notFound()
		}
		return nil
	case RoleManager, RoleCenterAdmin:
		return forbidden()
	default:
		return forbidden()
	}
}

// CanWriteSessionLog allows only the assigned trainer.
func CanWriteSessionLog(actor Actor, res Resource) error {
	membership, err := scoped(actor, res.FitnessCenterID, res.BranchID)
	if err != nil {
		return err
	}
	if membership.Role != RoleTrainer || res.TrainerUserID != actor.UserID {
		if membership.Role == RoleTrainer {
			return notFound()
		}
		return forbidden()
	}
	return nil
}

// CanManagePeople allows a manager to invite, assign, suspend, or remove a
// trainer or a client on one of their branches. The fitness center account
// can do that for a manager, a trainer, or a client. Neither can manage a
// center admin.
func CanManagePeople(actor Actor, centerID, branchID string, target Role) error {
	membership, err := scoped(actor, centerID, branchID)
	if err != nil {
		return err
	}
	switch membership.Role {
	case RoleManager:
		if target == RoleClient || target == RoleTrainer {
			return nil
		}
		return forbidden()
	case RoleCenterAdmin:
		if target == RoleClient || target == RoleTrainer || target == RoleManager {
			return nil
		}
		return forbidden()
	default:
		return forbidden()
	}
}

// CanManageBranches allows only the fitness center account to create or rename a branch.
func CanManageBranches(actor Actor, centerID string) error {
	membership, err := selectMembership(actor, centerID)
	if err != nil {
		return err
	}
	if membership.Role != RoleCenterAdmin {
		return forbidden()
	}
	return nil
}

func scoped(actor Actor, centerID, branchID string) (Membership, error) {
	membership, err := selectMembership(actor, centerID)
	if err != nil {
		return Membership{}, err
	}
	if !branchVisible(membership, branchID) {
		return Membership{}, notFound()
	}
	return membership, nil
}

func selectMembership(actor Actor, centerID string) (Membership, error) {
	if centerID == "" {
		active := activeMemberships(actor)
		switch len(active) {
		case 0:
			if len(suspendedMemberships(actor)) > 0 {
				return Membership{}, suspended()
			}
			return Membership{}, forbidden()
		case 1:
			return active[0], nil
		default:
			return Membership{}, chooseCenter()
		}
	}
	for _, membership := range actor.Memberships {
		if membership.FitnessCenterID != centerID {
			continue
		}
		if membership.Status == StatusSuspended {
			return Membership{}, suspended()
		}
		if membership.Status != StatusActive || !membership.Role.Valid() {
			return Membership{}, forbidden()
		}
		return membership, nil
	}
	return Membership{}, notFound()
}

func selectBranch(membership Membership, header string) (string, error) {
	switch membership.Role {
	case RoleClient, RoleTrainer:
		return membership.BranchID, nil
	case RoleManager:
		if header == "" {
			if len(membership.BranchIDs) == 1 {
				return membership.BranchIDs[0], nil
			}
			return "", chooseBranch()
		}
		if !contains(membership.BranchIDs, header) {
			return "", notFound()
		}
		return header, nil
	case RoleCenterAdmin:
		return header, nil
	default:
		return "", forbidden()
	}
}

func branchVisible(membership Membership, branchID string) bool {
	switch membership.Role {
	case RoleCenterAdmin:
		return true
	case RoleManager:
		if branchID == "" {
			return len(membership.BranchIDs) == 1
		}
		return contains(membership.BranchIDs, branchID)
	default:
		if branchID == "" {
			return membership.BranchID != ""
		}
		return membership.BranchID == branchID
	}
}

func activeMemberships(actor Actor) []Membership {
	var active []Membership
	for _, membership := range actor.Memberships {
		if membership.Status == StatusActive && membership.Role.Valid() {
			active = append(active, membership)
		}
	}
	return active
}

func suspendedMemberships(actor Actor) []Membership {
	var suspendedRows []Membership
	for _, membership := range actor.Memberships {
		if membership.Status == StatusSuspended {
			suspendedRows = append(suspendedRows, membership)
		}
	}
	return suspendedRows
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
