package main

type UpsertEvent struct {
	CreateService      bool
	PayloadPassthrough bool
	ServiceName        string
	ARN                string
	EnterpriseMeta     *EnterpriseMeta
}

func (e UpsertEvent) Identifier() string {
	return e.ARN
}

func (e UpsertEvent) Reconcile(env Environment) error {
	env.Logger.Info("Upserting Lambda", "arn", e.ARN)
	return nil
}
