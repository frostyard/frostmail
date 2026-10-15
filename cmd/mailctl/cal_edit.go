package main

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

type calFieldOptions struct {
	summary, location, notes, repeat string
	start, end, zone, date, endDate  string
	alerts                           []int
	invite, inviteOptional           []string
}

func (o *calFieldOptions) flags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&o.summary, "summary", "", "event summary (required)")
	f.StringVar(&o.location, "location", "", "event location")
	f.StringVar(&o.notes, "notes", "", "event description")
	f.StringVar(&o.repeat, "repeat", "", "recurrence rule (RRULE value)")
	f.IntSliceVar(&o.alerts, "alert", nil, "minutes before the start (repeatable)")
	f.StringArrayVar(&o.invite, "invite", nil, "required invitee address (repeatable)")
	f.StringArrayVar(&o.inviteOptional, "invite-optional", nil, "optional invitee address (repeatable)")
	f.StringVar(&o.start, "start", "", "start time (YYYY-MM-DD HH:MM or YYYY-MM-DDTHH:MM)")
	f.StringVar(&o.end, "end", "", "end time (default: an hour after the start)")
	f.StringVar(&o.zone, "zone", "", "time zone (IANA; default: the local zone)")
	f.StringVar(&o.date, "date", "", "all-day event's first day (YYYY-MM-DD)")
	f.StringVar(&o.endDate, "end-date", "", "all-day event's last day, inclusive (default: --date)")
}

func (o *calFieldOptions) fields(cmd *cobra.Command) (api.EventFields, error) {
	fields := api.EventFields{Summary: &o.summary}
	if strings.TrimSpace(o.summary) == "" {
		return fields, fmt.Errorf("summary is required and must not be blank")
	}
	f := cmd.Flags()
	if f.Changed("start") == f.Changed("date") {
		return fields, fmt.Errorf("exactly one of --start and --date is required")
	}
	for _, field := range []struct {
		name  string
		value *string
		dest  **string
	}{
		{"location", &o.location, &fields.Location},
		{"notes", &o.notes, &fields.Description},
		{"repeat", &o.repeat, &fields.Rrule},
	} {
		if f.Changed(field.name) {
			*field.dest = field.value
		}
	}
	if f.Changed("alert") {
		fields.Alarms = make([]int64, len(o.alerts))
		for i, alert := range o.alerts {
			fields.Alarms[i] = int64(alert)
		}
	}
	for _, email := range o.invite {
		fields.Attendees = append(fields.Attendees, api.Invitee{Email: email, Role: new(api.AttendeeRoleRequired)})
	}
	for _, email := range o.inviteOptional {
		fields.Attendees = append(fields.Attendees, api.Invitee{Email: email, Role: new(api.AttendeeRoleOptional)})
	}
	if f.Changed("date") {
		if f.Changed("end") || f.Changed("zone") {
			return fields, fmt.Errorf("--end and --zone require --start")
		}
		err := o.allDayFields(cmd, &fields)
		return fields, err
	}
	if f.Changed("end-date") {
		return fields, fmt.Errorf("--end-date requires --date")
	}
	err := o.timedFields(cmd, &fields)
	return fields, err
}

func (o *calFieldOptions) allDayFields(cmd *cobra.Command, fields *api.EventFields) error {
	start, err := time.Parse(time.DateOnly, o.date)
	if err != nil {
		return fmt.Errorf("invalid date %q: %w", o.date, err)
	}
	end := start
	if cmd.Flags().Changed("end-date") {
		end, err = time.Parse(time.DateOnly, o.endDate)
		if err != nil {
			return fmt.Errorf("invalid end date %q: %w", o.endDate, err)
		}
	}
	if end.Before(start) {
		return fmt.Errorf("end date must not be before date")
	}
	fields.AllDay = new(true)
	fields.StartDate = &o.date
	fields.EndDate = new(end.AddDate(0, 0, 1).Format(time.DateOnly))
	return nil
}

func (o *calFieldOptions) timedFields(cmd *cobra.Command, fields *api.EventFields) error {
	zone := o.zone
	if !cmd.Flags().Changed("zone") {
		zone = calLocalZone()
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return fmt.Errorf("invalid time zone %q: %w", zone, err)
	}
	start, err := calTime(o.start, loc)
	if err != nil {
		return err
	}
	end := start.Add(time.Hour)
	if cmd.Flags().Changed("end") {
		end, err = calTime(o.end, loc)
		if err != nil {
			return err
		}
	}
	if !end.After(start) {
		return fmt.Errorf("end must be after start")
	}
	fields.AllDay = new(false)
	fields.Start, fields.End, fields.TimeZone = &start, &end, &zone
	return nil
}

func calTime(value string, loc *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02T15:04", strings.Replace(value, " ", "T", 1), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q: %w", value, err)
	}
	return parsed, nil
}

func calLocalZone() string {
	if zone := strings.TrimPrefix(os.Getenv("TZ"), ":"); zone != "" {
		if _, err := time.LoadLocation(zone); err == nil {
			return zone
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
			return zone
		}
	}
	return "UTC"
}

func newCalAddCmd(opts *rootOptions) *cobra.Command {
	var calendar string
	var notify bool
	var fields calFieldOptions
	cmd := &cobra.Command{
		Use: "add [flags]", Short: "Add an event", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			calendarID, err := pimIDFlag(cmd, "calendar", calendar)
			if err != nil {
				return err
			}
			patch, err := fields.fields(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			p := &api.CalendarCreateParams{CalendarID: calendarID, Fields: patch}
			if notify {
				p.Notify = new(true)
			}
			result, err := c.Calendar().Create(ctx, p)
			if err != nil {
				return calWriteError(err)
			}
			if written, err := clix.OutputJSON(result); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "added %s (event %d)\n", result.Event.Summary, result.Event.ID)
			return err
		},
	}
	cmd.Flags().StringVar(&calendar, "calendar", "", "calendar ID")
	cmd.Flags().BoolVar(&notify, "notify", false, "send notifications to invitees")
	fields.flags(cmd)
	return cmd
}

func calDeleteParams(cmd *cobra.Command, id, occurrence, scope string) (*api.CalendarDeleteParams, error) {
	eventID, err := pimID(id, "event")
	if err != nil {
		return nil, err
	}
	p := &api.CalendarDeleteParams{ID: eventID}
	if cmd.Flags().Changed("occurrence") != cmd.Flags().Changed("scope") {
		return nil, fmt.Errorf("--occurrence and --scope must be given together")
	}
	if cmd.Flags().Changed("scope") {
		if occurrence == "" {
			return nil, fmt.Errorf("occurrence must not be empty")
		}
		s := api.EventScope(scope)
		switch s {
		case api.EventScopeThis, api.EventScopeFuture, api.EventScopeAll:
			p.RecurrenceID, p.Scope = &occurrence, &s
		default:
			return nil, fmt.Errorf("scope must be this, future or all")
		}
	}
	return p, nil
}

func newCalRmCmd(opts *rootOptions) *cobra.Command {
	var occurrence, scope string
	var notify bool
	cmd := &cobra.Command{
		Use: "rm EVENT_ID", Short: "Delete an event", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := calDeleteParams(cmd, args[0], occurrence, scope)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if notify {
				p.Notify = new(true)
			}
			result, err := c.Calendar().Delete(ctx, p)
			if err != nil {
				return calWriteError(err)
			}
			if written, err := clix.OutputJSON(result); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted event %d\n", p.ID)
			return err
		},
	}
	cmd.Flags().StringVar(&occurrence, "occurrence", "", "recurrence ID from agenda --ids")
	cmd.Flags().StringVar(&scope, "scope", "", "deletion scope: this, future or all (requires --occurrence)")
	cmd.Flags().BoolVar(&notify, "notify", false, "send notifications to invitees")
	return cmd
}

func calWriteError(err error) error {
	var rpcErr *api.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != api.CodeNotifyRequired {
		return err
	}
	var notice api.InviteNotice
	if decodeErr := json.Unmarshal(rpcErr.Data, &notice); decodeErr != nil {
		return err
	}
	return fmt.Errorf("the change sends %s to %d invitees: run it again with --notify to send them", notice.Kind, notice.Count)
}
