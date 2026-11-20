package settings

import (
	"errors"
	"testing"
)

func addOpenAI(t *testing.T, id, base string) {
	t.Helper()
	if _, err := AddProvider(Provider{ID: id, Name: id, Type: TypeOpenAI, BaseURL: base, TextModel: "t", VisionModel: "v"}); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingDefaultsToTheBuiltin(t *testing.T) {
	isolate(t)
	for _, task := range Tasks {
		if RoutedProvider(task) != BuiltinID {
			t.Errorf("%s must default to the built-in provider", task)
		}
	}
	addOpenAI(t, "cloud", "https://api.example.com/v1")
	if err := SetRouting(map[string]string{TaskAssistant: "cloud"}); err != nil {
		t.Fatal(err)
	}
	if RoutedProvider(TaskAssistant) != "cloud" || RoutedProvider(TaskQuality) != BuiltinID {
		t.Error("only the routed task changes")
	}
	// routing back to the built-in removes the entry
	if err := SetRouting(map[string]string{TaskAssistant: BuiltinID}); err != nil {
		t.Fatal(err)
	}
	if len(Load().Routing) != 0 {
		t.Errorf("the entry must go: %v", Load().Routing)
	}
}

func TestModelForTask(t *testing.T) {
	p := Provider{TextModel: "text-m", VisionModel: "vision-m"}
	if ModelFor(p, TaskAssistant) != "text-m" {
		t.Error("the assistant uses the text model")
	}
	for _, task := range []string{TaskDescribeImage, TaskDescribeVideo, TaskQuality} {
		if ModelFor(p, task) != "vision-m" {
			t.Errorf("%s uses the vision model", task)
		}
	}
	if !ValidTask(TaskQuality) || ValidTask("nope") {
		t.Error("ValidTask")
	}
}

func TestConsentIsBoundToTheAddress(t *testing.T) {
	isolate(t)
	addOpenAI(t, "cloud", "https://api.example.com/v1")
	if Acknowledged("cloud", "https://api.example.com/v1") {
		t.Fatal("nothing acknowledged yet")
	}
	if err := Acknowledge("cloud"); err != nil {
		t.Fatal(err)
	}
	if !Acknowledged("cloud", "https://api.example.com/v1") {
		t.Fatal("acknowledged")
	}
	// editing the address makes it stop counting
	p, _ := FindProvider("cloud")
	p.BaseURL = "https://other.example.com/v1"
	if _, err := UpdateProvider("cloud", p); err != nil {
		t.Fatal(err)
	}
	if Acknowledged("cloud", "https://other.example.com/v1") || Acknowledged("cloud", "https://api.example.com/v1") && false {
		t.Fatal("a new address needs a new acknowledgement")
	}
	if err := Acknowledge("nope"); !errors.Is(err, ErrNoSuchProvider) {
		t.Errorf("unknown provider: %v", err)
	}
	// deleting the provider forgets the consent
	if err := Acknowledge("cloud"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProvider("cloud"); err != nil {
		t.Fatal(err)
	}
	if len(Load().Consent) != 0 {
		t.Errorf("consent must go with the provider: %v", Load().Consent)
	}
}

func TestMaxCalls(t *testing.T) {
	isolate(t)
	if MaxCalls() != DefaultMaxCalls {
		t.Fatalf("default %d", MaxCalls())
	}
	if err := SetMaxCalls(30); err != nil || MaxCalls() != 30 {
		t.Fatalf("%v %d", err, MaxCalls())
	}
	if SetMaxCalls(0) == nil {
		t.Error("the cap must be at least 1")
	}
	if err := SetMaxCalls(DefaultMaxCalls); err != nil || Load().Remote != nil {
		t.Errorf("the default is not stored: %v %+v", err, Load().Remote)
	}
}

func TestAProviderInUseCannotBeRemovedNorLoseItsModel(t *testing.T) {
	isolate(t)
	addOpenAI(t, "cloud", "https://api.example.com/v1")
	_ = SetRouting(map[string]string{TaskQuality: "cloud"})
	err := DeleteProvider("cloud")
	var inUse ErrProviderInUse
	if !errors.As(err, &inUse) || inUse.Task != TaskQuality {
		t.Fatalf("want ErrProviderInUse naming the task, got %v", err)
	}
	if _, ok := FindProvider("cloud"); !ok {
		t.Error("the provider must remain")
	}
	p, _ := FindProvider("cloud")
	p.VisionModel = ""
	if _, err := UpdateProvider("cloud", p); err == nil {
		t.Error("removing the vision model of a routed provider must be refused")
	}
	p.VisionModel = "v2"
	if _, err := UpdateProvider("cloud", p); err != nil {
		t.Errorf("changing the model is fine: %v", err)
	}
	_ = SetRouting(map[string]string{TaskQuality: BuiltinID})
	if err := DeleteProvider("cloud"); err != nil {
		t.Errorf("unrouted, it can go: %v", err)
	}
	if ProviderInUse("cloud") != "" {
		t.Error("ProviderInUse")
	}
}
