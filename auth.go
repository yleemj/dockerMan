package dockerMan

type (
    Account struct {
        Username string       `json:"username,omitempty" gorethink:"username"`
        Password string       `json:"password,omitempty" gorethink:"password"`
    }
)
