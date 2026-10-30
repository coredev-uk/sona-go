import { AuthService, type AuthStatus, type SecondFactor } from "@bindings/services";
import { Browser } from "@wailsio/runtime";
import { Clipboard } from "@wailsio/runtime";
import {
  CircleAlert,
  ExternalLink,
  KeyRound,
  LoaderCircle,
  ShieldAlert,
  WifiOff,
} from "lucide-react";
import { useEffect, useReducer, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Button } from "@/shared/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { callFailure } from "@/shared/lib/errors";
import { cn } from "@/shared/lib/utils";

import { useAuth } from "./queries";
import {
  classifySignInError,
  errorCopy,
  initialSignIn,
  signInReducer,
  type DeliveryMode,
  type SignInError,
  type SignInStep,
} from "./signin";

const openURL = (url: string) => void Browser.OpenURL(url);

function Notice({
  icon,
  tone = "neutral",
  children,
  action,
}: {
  icon: ReactNode;
  tone?: "neutral" | "problem";
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      role={tone === "problem" ? "alert" : "status"}
      className={cn(
        "flex items-start gap-3 rounded-lg px-4 py-3",
        tone === "problem" ? "border border-control-line bg-raised" : "bg-hover",
      )}
    >
      <span className="mt-0.5 shrink-0">{icon}</span>
      <div className="min-w-0 flex-1 text-sub text-ink">{children}</div>
      {action && <div className="-my-1 shrink-0">{action}</div>}
    </div>
  );
}

/** Field with its label above and an optional hint tied by aria-describedby. */
function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <label htmlFor={id} className="text-ui font-medium text-ink">
        {label}
      </label>
      {children}
      {hint && (
        <p id={`${id}-hint`} className="text-meta text-ink-muted">
          {hint}
        </p>
      )}
    </div>
  );
}

const inputCls = "h-10 bg-raised text-ui";

/** An error from the sign-in flow: what happened, and the button for the next step. */
function SignInErrorNotice({ error, onBrowser }: { error: SignInError; onBrowser: () => void }) {
  const copy = errorCopy(error);
  let action: ReactNode = null;
  if (copy.next === "browser") {
    action = (
      <Button variant="outline" size="sm" onClick={onBrowser}>
        Sign in with your browser
      </Button>
    );
  } else if (copy.next === "account-site") {
    action = (
      <Button variant="outline" size="sm" onClick={() => openURL("https://account.apple.com")}>
        account.apple.com
        <ExternalLink strokeWidth={1.75} />
      </Button>
    );
  }
  return (
    <Notice
      tone="problem"
      icon={<CircleAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
      action={action}
    >
      <p className="font-medium">{copy.title}</p>
      <p className="mt-0.5 text-ink-muted">{copy.detail}</p>
      {error.code === "locked" && (
        <button
          type="button"
          onClick={() => openURL("https://iforgot.apple.com")}
          className="mt-1 text-ink underline underline-offset-2"
        >
          Unlock at iforgot.apple.com
        </button>
      )}
    </Notice>
  );
}

function CredentialsForm({
  step,
  dispatch,
  onBrowser,
}: {
  step: Extract<SignInStep, { kind: "credentials" }>;
  dispatch: (a: Parameters<typeof signInReducer>[1]) => void;
  onBrowser: () => void;
}) {
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!account.trim() || !password) return;
    dispatch({ type: "submit" });
    try {
      const factor = await AuthService.SignInWithPassword(account.trim(), password);
      if (factor) dispatch({ type: "second-factor", factor });
      else dispatch({ type: "signed-in" });
    } catch (err) {
      const error = classifySignInError(err);
      dispatch({ type: "failed", error });
    } finally {
      // The password is used for one exchange and never kept.
      setPassword("");
    }
  };

  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4" aria-busy={step.busy}>
      {step.error && <SignInErrorNotice error={step.error} onBrowser={onBrowser} />}
      <Field
        id="apple-account"
        label="Apple Account"
        hint="The email address or phone number you use with Apple."
      >
        <Input
          id="apple-account"
          aria-describedby="apple-account-hint"
          type="text"
          inputMode="email"
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          required
          value={account}
          onChange={(e) => setAccount(e.target.value)}
          aria-invalid={step.error?.code === "invalid_credentials" || undefined}
          className={inputCls}
        />
      </Field>
      <Field id="apple-password" label="Password">
        <Input
          id="apple-password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          aria-invalid={step.error?.code === "invalid_credentials" || undefined}
          className={inputCls}
        />
      </Field>
      <div className="flex items-center gap-3">
        <Button type="submit" disabled={step.busy || !account.trim() || !password}>
          {step.busy && <LoaderCircle className="animate-spin" />}
          {step.busy ? "Signing in…" : "Sign in"}
        </Button>
        <p className="text-meta text-ink-muted">
          Sona talks to Apple directly. Your password is used once and never saved.
        </p>
      </div>
    </form>
  );
}

function phoneLabel(factor: SecondFactor, phoneId: number): string {
  return factor.phones.find((p) => p.id === phoneId)?.number ?? "your phone";
}

function CodeForm({
  step,
  dispatch,
}: {
  step: Extract<SignInStep, { kind: "code" }>;
  dispatch: (a: Parameters<typeof signInReducer>[1]) => void;
}) {
  const [code, setCode] = useState("");
  const len = step.factor.codeLength || 6;
  const { phoneId, mode } = step.target;

  const submit = async (value: string) => {
    if (value.length !== len || step.busy) return;
    dispatch({ type: "submit" });
    try {
      await AuthService.SubmitSecurityCode(value, phoneId, mode);
      dispatch({ type: "signed-in" });
    } catch (err) {
      setCode("");
      dispatch({ type: "failed", error: classifySignInError(err) });
    }
  };
  const resend = async () => {
    dispatch({ type: "resend" });
    try {
      if (phoneId === 0) await AuthService.ResendSecurityCode();
      else await AuthService.SendCodeToPhone(phoneId, mode);
      dispatch({ type: "resent" });
    } catch (err) {
      dispatch({ type: "failed", error: classifySignInError(err) });
    }
  };
  const cancel = () => {
    void AuthService.CancelSignIn();
    dispatch({ type: "cancel" });
  };

  const where =
    phoneId === 0
      ? `Apple sent a ${len}-digit code to the devices signed in to this Apple Account. Enter it here to finish signing in.`
      : mode === "voice"
        ? `Apple is calling ${phoneLabel(step.factor, phoneId)} with a ${len}-digit code.`
        : `Apple sent a ${len}-digit code to ${phoneLabel(step.factor, phoneId)} by text message.`;

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit(code);
      }}
      className="flex flex-col gap-4"
      aria-busy={step.busy}
    >
      <div>
        <h3 className="text-heading text-ink">Enter the verification code</h3>
        <p className="mt-1 text-ui text-ink-muted">{where}</p>
      </div>
      {step.error && <SignInErrorNotice error={step.error} onBrowser={() => {}} />}
      {step.notice && !step.error && (
        <p role="status" className="text-sub text-ink">
          {step.notice}
        </p>
      )}
      <Field id="security-code" label="Code" hint={`${len} digits`}>
        <Input
          id="security-code"
          aria-describedby="security-code-hint"
          autoFocus
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="[0-9]*"
          maxLength={len}
          value={code}
          disabled={step.busy}
          aria-invalid={step.error?.code === "incorrect_code" || undefined}
          onChange={(e) => {
            const v = e.target.value.replace(/\D/g, "").slice(0, len);
            setCode(v);
            if (v.length === len) void submit(v);
          }}
          className={cn(inputCls, "w-48 text-center text-heading tracking-[0.4em] num")}
          style={{ width: `${len * 1.6 + 2}em` }}
        />
      </Field>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={step.busy || code.length !== len}>
          {step.busy && <LoaderCircle className="animate-spin" />}
          {step.busy ? "Checking…" : "Verify"}
        </Button>
        <Button type="button" variant="ghost" disabled={step.busy} onClick={() => void resend()}>
          {phoneId === 0 ? "Send a new code" : mode === "voice" ? "Call again" : "Text again"}
        </Button>
        {step.factor.phones.length > 0 && (
          <Button
            type="button"
            variant="ghost"
            disabled={step.busy}
            onClick={() => dispatch({ type: "choose-phone" })}
          >
            {phoneId === 0 ? "Get a code by text or call" : "Use another number"}
          </Button>
        )}
        <Button type="button" variant="ghost" className="ml-auto" onClick={cancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

function PhoneChoice({
  step,
  dispatch,
}: {
  step: Extract<SignInStep, { kind: "choose-phone" }>;
  dispatch: (a: Parameters<typeof signInReducer>[1]) => void;
}) {
  const phones = step.factor.phones;
  const [phoneId, setPhoneId] = useState(phones[0]?.id ?? 0);
  const [mode, setMode] = useState<DeliveryMode>(phones[0]?.mode === "voice" ? "voice" : "sms");

  const send = async () => {
    dispatch({ type: "send-to-phone", phoneId, mode });
    try {
      await AuthService.SendCodeToPhone(phoneId, mode);
      dispatch({ type: "sent", target: { phoneId, mode } });
    } catch (err) {
      dispatch({ type: "failed", error: classifySignInError(err) });
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h3 className="text-heading text-ink">Where should Apple send the code?</h3>
        <p className="mt-1 text-ui text-ink-muted">
          Choose one of the trusted phone numbers on this Apple Account.
        </p>
      </div>
      {step.error && <SignInErrorNotice error={step.error} onBrowser={() => {}} />}
      <fieldset className="flex flex-col gap-1">
        <legend className="sr-only">Trusted phone number</legend>
        {phones.map((p) => (
          <label
            key={p.id}
            className="flex h-10 cursor-pointer items-center gap-3 rounded-md px-3 text-ui text-ink hover:bg-hover has-checked:bg-active"
          >
            <input
              type="radio"
              name="phone"
              value={p.id}
              checked={phoneId === p.id}
              onChange={() => setPhoneId(p.id)}
              className="accent-[var(--ink)]"
            />
            <span className="num">{p.number}</span>
          </label>
        ))}
      </fieldset>
      <fieldset className="flex items-center gap-1 self-start rounded-md bg-hover p-1">
        <legend className="sr-only">How to receive the code</legend>
        {(
          [
            ["sms", "Text message"],
            ["voice", "Phone call"],
          ] as const
        ).map(([m, label]) => (
          <label
            key={m}
            className="flex h-7 cursor-pointer items-center rounded-sm px-3 text-sub text-ink-muted has-checked:bg-raised has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-ring"
          >
            <input
              type="radio"
              name="mode"
              value={m}
              checked={mode === m}
              onChange={() => setMode(m)}
              className="sr-only"
            />
            {label}
          </label>
        ))}
      </fieldset>
      <div className="flex items-center gap-2">
        <Button onClick={() => void send()} disabled={step.busy || !phoneId}>
          {step.busy && <LoaderCircle className="animate-spin" />}
          {step.busy ? "Sending…" : "Send code"}
        </Button>
        <Button
          variant="ghost"
          onClick={() => {
            void AuthService.CancelSignIn();
            dispatch({ type: "cancel" });
          }}
        >
          Cancel
        </Button>
      </div>
    </div>
  );
}

/** The browser fallback while Apple's page is open in the system browser. */
function BrowserWaiting({ status }: { status: AuthStatus }) {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <LoaderCircle
          aria-hidden
          className="size-5 animate-spin text-ink-muted"
          strokeWidth={1.75}
        />
        <div>
          <h3 className="text-heading text-ink">Finish signing in in your browser</h3>
          <p className="mt-0.5 text-ui text-ink-muted">
            Apple's sign-in page is open in your browser. Sona continues by itself when you're done.
          </p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {status.signInUrl && (
          <>
            <Button variant="outline" size="sm" onClick={() => openURL(status.signInUrl)}>
              Open the page again
              <ExternalLink strokeWidth={1.75} />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() =>
                void Clipboard.SetText(status.signInUrl).then(() => toast("Sign-in link copied"))
              }
            >
              Copy link
            </Button>
          </>
        )}
        <Button variant="ghost" size="sm" onClick={() => void AuthService.CancelSignIn()}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

/** Manual token paste: the last-resort fallback for developers and odd accounts. */
function TokenPaste() {
  const [open, setOpen] = useState(false);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await AuthService.SignInWithToken(token.trim());
      setToken("");
    } catch (err) {
      const { code, message } = callFailure(err);
      setError(
        code === "mut_rejected" || code === "unauthorized"
          ? "Apple didn't accept that token. Copy a fresh one from a signed-in Apple Music web session."
          : code === "network"
            ? "Couldn't reach Apple to check the token. Check your connection and try again."
            : message,
      );
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex items-center gap-2 text-ui text-ink-muted underline-offset-2 hover:text-ink hover:underline"
      >
        <KeyRound aria-hidden className="size-4" strokeWidth={1.75} />
        Paste a Music User Token
      </button>
    );
  }
  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-3">
      <Field
        id="mut"
        label="Music User Token"
        hint="The token from a signed-in Apple Music web session. Sona checks it with Apple and keeps it in your keyring."
      >
        <textarea
          id="mut"
          aria-describedby="mut-hint"
          rows={3}
          spellCheck={false}
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          className="w-full resize-none rounded-md border border-control-line bg-raised px-3 py-2 font-[inherit] text-sub break-all text-ink outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        />
      </Field>
      {error && (
        <p role="alert" className="flex items-start gap-2 text-sub text-ink">
          <CircleAlert
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-danger"
            strokeWidth={1.75}
          />
          {error}
        </p>
      )}
      <div className="flex gap-2">
        <Button
          type="submit"
          variant="outline"
          size="sm"
          disabled={busy || token.trim().length < 20}
        >
          {busy ? "Checking…" : "Use this token"}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setOpen(false)}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

function SignInFlow({ status }: { status: AuthStatus }) {
  const [step, dispatch] = useReducer(signInReducer, initialSignIn);

  // Keep the form in step with Go: restored challenges, timeouts, sign-in elsewhere.
  useEffect(() => {
    dispatch({
      type: "status",
      state: status.state,
      factor: status.secondFactor,
      lastError: status.lastError,
      lastErrorCode: status.lastErrorCode,
    });
  }, [status.state, status.secondFactor, status.lastError, status.lastErrorCode]);

  const browser = async () => {
    dispatch({ type: "cancel" });
    try {
      await AuthService.SignIn();
    } catch (err) {
      const error = classifySignInError(err);
      if (error.code !== "cancelled") dispatch({ type: "failed", error });
    }
  };

  const browserPending =
    status.state === "signing_in" && !status.secondFactor && step.kind === "credentials";

  return (
    <div className="rounded-lg bg-surface p-6">
      {browserPending ? (
        <BrowserWaiting status={status} />
      ) : step.kind === "code" ? (
        <CodeForm step={step} dispatch={dispatch} />
      ) : step.kind === "choose-phone" ? (
        <PhoneChoice step={step} dispatch={dispatch} />
      ) : step.kind === "done" ? (
        <p className="flex items-center gap-2 text-ui text-ink-muted">
          <LoaderCircle aria-hidden className="size-4 animate-spin" />
          Checking your account with Apple…
        </p>
      ) : (
        <>
          <div className="mb-5">
            <h3 className="text-heading text-ink">Sign in to Apple Music</h3>
            <p className="mt-1 text-ui text-ink-muted">
              Use the Apple Account with your Apple Music subscription.
            </p>
          </div>
          <CredentialsForm step={step} dispatch={dispatch} onBrowser={() => void browser()} />
        </>
      )}
      {step.kind === "credentials" && !browserPending && (
        <div className="mt-6 flex flex-col items-start gap-3 border-t border-line pt-5">
          <p className="text-sub text-ink-muted">
            Use a passkey, a security key or a browser you're already signed in to:
          </p>
          <Button variant="outline" size="sm" onClick={() => void browser()} disabled={step.busy}>
            Sign in with your browser
            <ExternalLink strokeWidth={1.75} />
          </Button>
          <TokenPaste />
        </div>
      )}
    </div>
  );
}

function SignedIn({ status }: { status: AuthStatus }) {
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);

  const signOut = async () => {
    setBusy(true);
    try {
      await AuthService.SignOut();
      setConfirm(false);
    } catch (err) {
      toast.error("Couldn't sign out", { description: callFailure(err).message });
    } finally {
      setBusy(false);
    }
  };
  const revalidate = async () => {
    setChecking(true);
    try {
      await AuthService.Revalidate();
    } catch (err) {
      toast.error("Couldn't reach Apple", { description: callFailure(err).message });
    } finally {
      setChecking(false);
    }
  };

  const store = status.storefrontName || status.storefrontId.toUpperCase();
  return (
    <div className="flex flex-col gap-4">
      {status.offline && (
        <Notice
          icon={<WifiOff aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />}
          action={
            <Button variant="ghost" size="sm" onClick={() => void revalidate()} disabled={checking}>
              {checking ? "Checking…" : "Check now"}
            </Button>
          }
        >
          Sona couldn't reach Apple to confirm your sign-in. It keeps using your saved token, and
          retries by itself.
        </Notice>
      )}
      <div className="rounded-lg bg-surface px-5">
        <div className="flex items-center justify-between gap-6 border-b border-line py-4">
          <div>
            <p className="text-ui font-medium text-ink">
              {status.state === "validating"
                ? "Checking your sign-in…"
                : "Signed in to Apple Music"}
            </p>
            <p className="mt-0.5 text-sub text-ink-muted">
              {store ? `Storefront: ${store}` : "Storefront unknown until Apple confirms the token"}
              {status.language && ` · ${status.language}`}
            </p>
          </div>
          <Button variant="destructive" size="sm" onClick={() => setConfirm(true)}>
            Sign out
          </Button>
        </div>
        {status.restricted && (
          <div className="border-b border-line py-4 text-sub text-ink-muted">
            Explicit content is restricted on this account; Sona hides it.
          </div>
        )}
        <div className="flex items-start justify-between gap-6 py-4">
          <div>
            <p className="text-ui font-medium text-ink">Where your token is kept</p>
            <p className="mt-0.5 text-sub text-ink-muted">{status.backend || "Unknown"}</p>
          </div>
        </div>
      </div>
      {status.unencrypted && (
        <Notice
          tone="problem"
          icon={<ShieldAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
        >
          <p className="font-medium">Your token is stored unencrypted</p>
          <p className="mt-0.5 text-ink-muted">
            No Secret Service keyring is running, so Sona keeps your Apple Music token in a file
            only your user can read. Start GNOME Keyring or KWallet and Sona moves it into the
            keyring.
          </p>
        </Notice>
      )}
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent showCloseButton={false} className="max-w-md">
          <DialogHeader>
            <DialogTitle>Sign out of Apple Music?</DialogTitle>
            <DialogDescription>
              Sona removes your token from the keyring, stops playback and deletes the copy of your
              library on this computer. Your Apple Music library itself isn't changed.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirm(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={() => void signOut()} disabled={busy}>
              {busy ? "Signing out…" : "Sign out"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export function AccountSection() {
  const auth = useAuth();
  if (auth.error) {
    return (
      <ErrorBlock
        title="Couldn't read the sign-in state"
        error={auth.error}
        onRetry={() => void auth.refetch()}
        retrying={auth.isRefetching}
      />
    );
  }
  const status = auth.data;
  if (!status || status.state === "loading") {
    return <div aria-busy className="h-48 rounded-lg bg-surface" />;
  }
  if (status.state === "signed_in" || status.state === "validating") {
    return <SignedIn status={status} />;
  }
  return (
    <div className="flex flex-col gap-4">
      {status.state === "needs_reauth" && (
        <Notice
          icon={<CircleAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
        >
          <p className="font-medium">Apple stopped accepting Sona's sign-in</p>
          <p className="mt-0.5 text-ink-muted">
            This happens after a password change or when the token expires. Your library on this
            computer is still here; sign in again to keep syncing and playing.
          </p>
        </Notice>
      )}
      {status.state === "error" && (
        <Notice
          icon={<CircleAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
        >
          <p className="font-medium">Apple Music isn't accepting Sona's requests</p>
          <p className="mt-0.5 text-ink-muted">
            {status.lastError ? `${status.lastError.replace(/^[a-z]+: /, "")}. ` : ""}
            Sona refetches its developer token by itself; if this persists, Apple changed its web
            player.
          </p>
        </Notice>
      )}
      {status.offline && (
        <Notice icon={<WifiOff aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />}>
          You're offline. Signing in needs a connection to Apple.
        </Notice>
      )}
      <SignInFlow status={status} />
    </div>
  );
}
