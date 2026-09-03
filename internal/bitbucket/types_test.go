package bitbucket

import (
	"encoding/json"
	"testing"
)

func TestRepository_JSONDeserialization(t *testing.T) {
	raw := `{
		"slug": "my-repo",
		"name": "My Repo",
		"full_name": "workspace/my-repo",
		"mainbranch": {"name": "main", "type": "branch"},
		"updated_on": "2024-01-15T10:00:00Z"
	}`

	var repo Repository
	if err := json.Unmarshal([]byte(raw), &repo); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if repo.Slug != "my-repo" {
		t.Errorf("Slug = %q, want %q", repo.Slug, "my-repo")
	}
	if repo.Name != "My Repo" {
		t.Errorf("Name = %q, want %q", repo.Name, "My Repo")
	}
	if repo.FullName != "workspace/my-repo" {
		t.Errorf("FullName = %q, want %q", repo.FullName, "workspace/my-repo")
	}
	if repo.MainBranch == nil {
		t.Fatal("MainBranch is nil, want non-nil")
	}
	if repo.MainBranch.Name != "main" {
		t.Errorf("MainBranch.Name = %q, want %q", repo.MainBranch.Name, "main")
	}
	if repo.MainBranch.Type != "branch" {
		t.Errorf("MainBranch.Type = %q, want %q", repo.MainBranch.Type, "branch")
	}
	if repo.UpdatedOn != "2024-01-15T10:00:00Z" {
		t.Errorf("UpdatedOn = %q, want %q", repo.UpdatedOn, "2024-01-15T10:00:00Z")
	}
}

func TestRepository_NilMainBranch(t *testing.T) {
	raw := `{"slug": "bare-repo", "name": "Bare"}`
	var repo Repository
	if err := json.Unmarshal([]byte(raw), &repo); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if repo.MainBranch != nil {
		t.Errorf("expected nil MainBranch for missing field, got %+v", repo.MainBranch)
	}
}

func TestBranch_JSONDeserialization(t *testing.T) {
	raw := `{"name": "feature/test", "target": {"hash": "abc123def456"}}`
	var branch Branch
	if err := json.Unmarshal([]byte(raw), &branch); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if branch.Name != "feature/test" {
		t.Errorf("Name = %q, want %q", branch.Name, "feature/test")
	}
	if branch.Target.Hash != "abc123def456" {
		t.Errorf("Target.Hash = %q, want %q", branch.Target.Hash, "abc123def456")
	}
}

func TestCreateBranchRequest_JSONSerialization(t *testing.T) {
	req := CreateBranchRequest{
		Name:   "feature/my-branch",
		Target: BranchTarget{Hash: "main"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded["name"] != "feature/my-branch" {
		t.Errorf("name = %v, want %q", decoded["name"], "feature/my-branch")
	}
	target, ok := decoded["target"].(map[string]interface{})
	if !ok {
		t.Fatal("target field is missing or wrong type")
	}
	if target["hash"] != "main" {
		t.Errorf("target.hash = %v, want %q", target["hash"], "main")
	}
}

func TestPaginatedResponse_JSONDeserialization(t *testing.T) {
	raw := `{
		"values": [
			{"slug": "repo-a", "name": "Repo A"},
			{"slug": "repo-b", "name": "Repo B"}
		],
		"next": "https://api.bitbucket.org/2.0/repositories/ws?page=2",
		"page": 1,
		"size": 2
	}`

	var page PaginatedResponse
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(page.Values) != 2 {
		t.Errorf("len(Values) = %d, want 2", len(page.Values))
	}
	if page.Values[0].Slug != "repo-a" {
		t.Errorf("Values[0].Slug = %q, want %q", page.Values[0].Slug, "repo-a")
	}
	if page.Next != "https://api.bitbucket.org/2.0/repositories/ws?page=2" {
		t.Errorf("Next = %q, unexpected", page.Next)
	}
	if page.Page != 1 {
		t.Errorf("Page = %d, want 1", page.Page)
	}
	if page.Size != 2 {
		t.Errorf("Size = %d, want 2", page.Size)
	}
}

func TestPaginatedResponse_EmptyValues(t *testing.T) {
	raw := `{"values": [], "next": "", "page": 1, "size": 0}`
	var page PaginatedResponse
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(page.Values) != 0 {
		t.Errorf("expected 0 values, got %d", len(page.Values))
	}
	if page.Next != "" {
		t.Errorf("Next = %q, want empty", page.Next)
	}
}

func TestAPIError_JSONDeserialization(t *testing.T) {
	raw := `{
		"type": "error",
		"status": 404,
		"error": {
			"message": "Resource not found",
			"detail": "The repository does not exist"
		}
	}`

	var apiErr APIError
	if err := json.Unmarshal([]byte(raw), &apiErr); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if apiErr.Type != "error" {
		t.Errorf("Type = %q, want %q", apiErr.Type, "error")
	}
	if apiErr.Status != 404 {
		t.Errorf("Status = %d, want 404", apiErr.Status)
	}
	if apiErr.Error.Message != "Resource not found" {
		t.Errorf("Error.Message = %q, want %q", apiErr.Error.Message, "Resource not found")
	}
	wantDetail := `"The repository does not exist"`
	if string(apiErr.Error.Detail) != wantDetail {
		t.Errorf("Error.Detail = %s, want %s", string(apiErr.Error.Detail), wantDetail)
	}
}

func TestAPIError_EmptyMessage(t *testing.T) {
	raw := `{"type": "error", "status": 500, "error": {}}`
	var apiErr APIError
	if err := json.Unmarshal([]byte(raw), &apiErr); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if apiErr.Error.Message != "" {
		t.Errorf("expected empty message, got %q", apiErr.Error.Message)
	}
}

func TestPRUpdateRequest_IncludesEmptyEditedBody(t *testing.T) {
	empty := ""
	data, err := json.Marshal(PRUpdateRequest{Description: &empty})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		Description *string `json:"description"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Description == nil || *payload.Description != "" {
		t.Fatalf("description = %v, want an included empty string", payload.Description)
	}
}
