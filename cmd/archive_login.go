package cmd

import (
	"fmt"

	"github.com/asciimoo/hister/server/archiver"

	"github.com/spf13/cobra"
)

var archiveLoginCmd = &cobra.Command{
	Use:   "archive-login [URL]",
	Short: "Sign in to sites in the archiver's browser profile",
	Long: "Open a visible browser with your archiver profile (archiver.profile_dir) so you can sign in to sites. " +
		"Close the browser window when you are done; the archiver then saves pages with these logins. " +
		"Run it on a machine with a display, then copy the profile directory to the server if Hister runs elsewhere. " +
		"Stop the server or wait for running archives first, because a profile can only be used by one browser at a time.",
	Args:             cobra.MaximumNArgs(1),
	PersistentPreRun: skipRuntimeInitialization,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		c, _, err := inspectConfig()
		if err != nil {
			return err
		}
		userID, _ := cmd.Flags().GetUint("user")
		dir := archiver.UserProfileDir(c.Archiver.ProfileDir, userID)
		if dir == "" {
			return fmt.Errorf("set archiver.profile_dir in the configuration first")
		}
		startURL := ""
		if len(args) == 1 {
			startURL = args[0]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Opening a browser with profile %s. Close it when you have signed in.\n", dir)
		a := archiver.New(c.Archiver, "")
		if err := a.Login(cmd.Context(), dir, startURL); err != nil {
			return fmt.Errorf("browser login failed: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Browser profile saved.")
		return err
	},
}

func init() {
	archiveLoginCmd.Flags().Uint("user", 0, "User ID whose profile to open (0 in single user mode)")
	rootCmd.AddCommand(archiveLoginCmd)
}
