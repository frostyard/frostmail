-- Whether the user chose a collection as the default of its kind
-- (account.setCollection). A default the user did not choose follows the
-- server's (RFC 6638's schedule-default-calendar-URL) when it names one;
-- before this, the first collection the server listed became the default.
ALTER TABLE collections ADD COLUMN default_chosen INTEGER NOT NULL DEFAULT 0 CHECK (default_chosen IN (0, 1));
