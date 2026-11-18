package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

// PromptTerminal reports whether standard input and standard error are both
// a terminal, so rzv may ask the person a question on stderr and read the
// answer. A character device that is not a terminal, such as /dev/null, does
// not count. cmd/rzv passes it as Deps.PromptTerminal.
func PromptTerminal() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stderr)
}

// maxStepUpCodeEntries is how many rejected codes rzv accepts: the service burns a
// code after five wrong ones.
const maxStepUpCodeEntries = 5

const reauthFallback = "This action needs a fresh confirmation. If you sign in with GitHub, Google or another provider, it needs a code sent to your email, which rzv cannot ask for here: do it in the Runzivo portal. Otherwise run `rzv login`, then retry."

const reauthFallbackFR = "Cette action demande une nouvelle confirmation. Si vous vous connectez avec GitHub, Google ou un autre fournisseur, elle demande un code envoyé à votre courriel, que rzv ne peut pas demander ici : faites-la dans le portail Runzivo. Sinon, lancez `rzv login`, puis réessayez."

// reauthFallbackError is the general advice in English, for apiError.
func reauthFallbackError() *Error {
	return newError("reauth_required", exitAuth, "%s", reauthFallback)
}

func (a *app) reauthError() *Error {
	return newError("reauth_required", exitAuth, "%s", a.text(reauthFallback, reauthFallbackFR))
}

func reauthRequired(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.Status == 403 && apiErr.Code == "reauth_required"
}

// withStepUp runs call, a request the service gates with a step-up in
// category. When the service answers 403 reauth_required to a person in a
// terminal, rzv emails a one-time code, asks for it, and runs call again
// once. call must send the same request each time (same idempotency or
// operation id). Errors from call come back unchanged for the command's own
// mapping; step-up refusals come back as *Error.
func (a *app) withStepUp(cmd *command, client *api.Client, category string, call func() error) error {
	err := call()
	if !reauthRequired(err) || a.apiKey() != "" {
		return err
	}
	if cmd.json() || a.Stdin == nil || a.PromptTerminal == nil || !a.PromptTerminal() {
		return newError("reauth_required", exitAuth, "%s %s", a.text(reauthFallback, reauthFallbackFR),
			a.text("To enter an emailed code in rzv instead, run the command again in a terminal without --json.",
				"Pour saisir dans rzv un code reçu par courriel, relancez plutôt la commande dans un terminal, sans --json."))
	}
	if err := a.stepUpEmailCode(client, category); err != nil {
		return err
	}
	return call()
}

// stepUpEmailCode asks the service for a code, reads it from the person and
// verifies it. The code is never printed, logged or stored.
func (a *app) stepUpEmailCode(client *api.Client, category string) error {
	availability, err := client.StepUpEmailCodeAvailability(a.ctx, category)
	if err != nil {
		return a.stepUpFailure(err)
	}
	if !availability.Available {
		if availability.Reason == "no_verified_email" {
			return a.noVerifiedEmailError()
		}
		return a.reauthError()
	}
	input := bufio.NewReader(io.LimitReader(a.Stdin, 4<<10))
	resent := false
	for {
		challenge, err := a.sendStepUpCode(client, category, availability.MaskedEmail)
		if err != nil {
			return err
		}
		expired, err := a.enterStepUpCode(client, input, challenge.ChallengeID)
		if err != nil || !expired {
			return err
		}
		if resent {
			return a.codeExpiredError()
		}
		yes, err := a.confirm(input, a.text("The code expired. Send a new code? [y/N] ", "Le code a expiré. Envoyer un nouveau code? [o/N] "))
		if err != nil {
			return err
		}
		if !yes {
			return a.codeExpiredError()
		}
		resent = true
	}
}

func (a *app) codeExpiredError() *Error {
	return newError("step_up_code_expired", exitAuth, "%s", a.text("The code expired. Run the command again to get a new one.", "Le code a expiré. Relancez la commande pour en obtenir un nouveau."))
}

func (a *app) stepUpCancelledError() *Error {
	return newError("step_up_cancelled", exitAuth, "%s", a.text("No code was entered. Nothing was changed.", "Aucun code n’a été saisi. Rien n’a été modifié."))
}

func (a *app) sendStepUpCode(client *api.Client, category, masked string) (*api.StepUpCodeChallenge, error) {
	if masked == "" {
		fmt.Fprintln(a.Stderr, a.text("This action needs a fresh confirmation. Sending a code to your verified email address.", "Cette action demande une nouvelle confirmation. Envoi d’un code à votre adresse courriel vérifiée."))
	} else {
		fmt.Fprintf(a.Stderr, a.text("This action needs a fresh confirmation. Sending a code to %s.\n", "Cette action demande une nouvelle confirmation. Envoi d’un code à %s.\n"), display(masked))
	}
	challenge, err := client.RequestStepUpEmailCode(a.ctx, category)
	if err != nil {
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == 429 && apiErr.Code == "rate_limited" {
			message := a.text("Too many codes were requested. Run the command again later.", "Trop de codes ont été demandés. Relancez la commande plus tard.")
			if apiErr.RetryAfter > 0 {
				message = fmt.Sprintf(a.text("Too many codes were requested. Wait %d seconds, then run the command again.", "Trop de codes ont été demandés. Attendez %d secondes, puis relancez la commande."), int(apiErr.RetryAfter.Seconds()))
			}
			return nil, newError("rate_limited", exitFailure, "%s", message)
		}
		return nil, a.stepUpFailure(err)
	}
	return challenge, nil
}

// enterStepUpCode reads codes until one is right. expired reports that the
// service said the code expired, so the caller may offer a new one. Only
// codes the service rejects count towards the limit.
func (a *app) enterStepUpCode(client *api.Client, input *bufio.Reader, challengeID string) (expired bool, err error) {
	for tries := 0; tries < maxStepUpCodeEntries; {
		fmt.Fprint(a.Stderr, a.text("Enter the 6-digit code from the email: ", "Saisissez le code à 6 chiffres reçu par courriel : "))
		code, ok := a.readLine(input)
		if !ok || code == "" {
			fmt.Fprintln(a.Stderr)
			return false, a.stepUpCancelledError()
		}
		if !sixDigits(code) {
			fmt.Fprintln(a.Stderr, a.text("The code is 6 digits.", "Le code compte 6 chiffres."))
			continue
		}
		_, err := client.VerifyStepUpEmailCode(a.ctx, challengeID, code)
		var apiErr *api.Error
		switch {
		case err == nil:
			return false, nil
		case !errors.As(err, &apiErr):
			return false, err
		case apiErr.Status == 0 || apiErr.Status >= 500:
			return false, apiError(err, "")
		case apiErr.Status == 400 && apiErr.Code == "step_up_code_invalid":
			tries++
			fmt.Fprintln(a.Stderr, a.text("That code is not right. Try again.", "Ce code n’est pas le bon. Réessayez."))
		case apiErr.Status == 410 && apiErr.Code == "step_up_code_expired":
			return true, nil
		case apiErr.Status == 429 && apiErr.Code == "step_up_attempts_exceeded":
			return false, newError(apiErr.Code, exitAuth, "%s", a.text("Too many wrong codes. Run the command again to get a new code.", "Trop de codes erronés. Relancez la commande pour obtenir un nouveau code."))
		default:
			return false, a.stepUpFailure(err)
		}
	}
	return false, newError("step_up_attempts_exceeded", exitAuth, "%s", a.text("Too many wrong codes. Run the command again to get a new code.", "Trop de codes erronés. Relancez la commande pour obtenir un nouveau code."))
}

// stepUpFailure maps a refusal from the step-up operations. A service
// without them (404), an unavailable one, or a refusal without a fix the
// person can make gets the general advice.
func (a *app) stepUpFailure(err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == 401:
		return apiError(err, "")
	case apiErr.Status == 403 && apiErr.Code == "step_up_no_verified_email":
		return a.noVerifiedEmailError()
	}
	return a.reauthError()
}

func (a *app) noVerifiedEmailError() *Error {
	return newError("step_up_no_verified_email", exitAuth, "%s", a.text(
		"This action needs a code sent to your email, but your account has no verified email address. Confirm your email in Sign-in methods in the Runzivo portal, then retry.",
		"Cette action demande un code envoyé par courriel, mais votre compte n’a aucune adresse courriel vérifiée. Confirmez votre courriel dans Méthodes de connexion du portail Runzivo, puis réessayez."))
}

// confirm asks a yes/no question on stderr; anything but yes is no. Ctrl-C
// cancels the command.
func (a *app) confirm(input *bufio.Reader, question string) (bool, error) {
	fmt.Fprint(a.Stderr, question)
	answer, _ := a.readLine(input)
	if a.cancelled() {
		fmt.Fprintln(a.Stderr)
		return false, a.stepUpCancelledError()
	}
	switch strings.ToLower(answer) {
	case "y", "yes", "o", "oui":
		return true, nil
	}
	return false, nil
}

// readLine reads one line, or gives up when the command is cancelled
// (Ctrl-C): a blocked terminal read does not watch the context, so it runs
// in a goroutine that the process abandons on exit.
func (a *app) readLine(input *bufio.Reader) (string, bool) {
	type result struct {
		line string
		ok   bool
	}
	done := make(chan result, 1)
	go func() {
		line, err := input.ReadString('\n')
		done <- result{strings.TrimSpace(line), err == nil || line != ""}
	}()
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case r := <-done:
		return r.line, r.ok
	case <-ctx.Done():
		return "", false
	}
}

func (a *app) cancelled() bool {
	return a.ctx != nil && a.ctx.Err() != nil
}

func sixDigits(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
