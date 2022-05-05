package main

type DeleteEvent struct {
	ServiceName    string
	EnterpriseMeta *EnterpriseMeta
}

func (e DeleteEvent) Identifier() string {
	return e.ServiceName
}

func (e DeleteEvent) Reconcile(env Environment) error {
	env.Logger.Info("Deleting Lambda", "service-name", e.ServiceName)
	return nil
}
