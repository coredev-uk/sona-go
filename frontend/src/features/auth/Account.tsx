import { AuthService, type AuthStatus, type SecondFactor } from "@bindings/services";
import { Browser } from "@wailsio/runtime";
import { Clipboard } from "@wailsio/runtime";
import { plural, t as coreT } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import {
  CircleAlert,
  ExternalLink,
  KeyRound,
  LoaderCircle,
  MessageSquareText,
  MonitorSmartphone,
  Phone,
  ShieldAlert,
  WifiOff,
} from "lucide-react";
import { useEffect, useReducer, useState, type FormEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { toast } from "sonner";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Button, buttonVariants } from "@/shared/ui/button";
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

import { KeyringRetry } from "./KeyringRetry";
import { useAuth } from "./queries";
import {
  classifySignInError,
  currentMethod,
  errorCopy,
  initialSignIn,
  signInReducer,
  type CodeMethod,
  type DeliveryMode,
  type SignInError,
  type SignInStep,
} from "./signin";

const openURL = (url: string) => void Browser.OpenURL(url);

/** The sign-in flow's titles: h3, under the account dialog's (or the setup's) title. */
function FlowTitle({ children }: { children: ReactNode }) {
  return <h3 className="text-heading text-ink">{children}</h3>;
}

function Notice({
  icon,
  tone = "neutral",
  live,
  children,
  action,
}: {
  icon: ReactNode;
  tone?: "neutral" | "problem";
  /** Overrides the role the tone implies: "status" for a problem shown on arrival, not caused by the user's last action. */
  live?: "status" | "alert";
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      role={live ?? (tone === "problem" ? "alert" : "status")}
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

const INPUT_CLS = "h-10 bg-raised text-ui";

/** An error from the sign-in flow: what happened, and the button for the next step. */
function SignInErrorNotice({ error, onBrowser }: { error: SignInError; onBrowser: () => void }) {
  const copy = errorCopy(error);
  let action: ReactNode = null;
  if (copy.next === "browser") {
    action = (
      <Button variant="outline" size="sm" onClick={onBrowser}>
        <Trans>Sign in with your browser</Trans>
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
          <Trans>Unlock at iforgot.apple.com</Trans>
        </button>
      )}
    </Notice>
  );
}

const FORM_ID = "apple-sign-in";

function CredentialsForm({
  step,
  dispatch,
  onBrowser,
  actions,
}: {
  step: Extract<SignInStep, { kind: "credentials" }>;
  dispatch: (a: Parameters<typeof signInReducer>[1]) => void;
  onBrowser: () => void;
  actions?: HTMLElement | null;
}) {
  const { t } = useLingui();
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

  const signIn = (
    <Button
      type="submit"
      form={FORM_ID}
      size={actions ? "sm" : "default"}
      disabled={step.busy || !account.trim() || !password}
    >
      {step.busy && <LoaderCircle className="animate-spin" />}
      {step.busy ? <Trans>Signing in…</Trans> : <Trans>Sign in</Trans>}
    </Button>
  );

  return (
    <form
      id={FORM_ID}
      onSubmit={(e) => void submit(e)}
      className="flex flex-col gap-4"
      aria-busy={step.busy}
    >
      {step.error && <SignInErrorNotice error={step.error} onBrowser={onBrowser} />}
      <Field
        id="apple-account"
        label={t`Apple Account`}
        hint={t`The email address or phone number you use with Apple.`}
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
          className={INPUT_CLS}
        />
      </Field>
      <Field id="apple-password" label={t`Password`}>
        <Input
          id="apple-password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          aria-invalid={step.error?.code === "invalid_credentials" || undefined}
          className={INPUT_CLS}
        />
      </Field>
      <div className="flex items-center gap-3">
        {/* With `actions`, Sign in sits in the host's foot (still this form's submit). */}
        {actions ? createPortal(signIn, actions) : signIn}
        <p className="text-meta text-ink-muted">
          <Trans>Sona talks to Apple directly. Your password is used once and never saved.</Trans>
        </p>
      </div>
    </form>
  );
}

function phoneLabel(factor: SecondFactor, phoneId: number): string {
  return factor.phones.find((p) => p.id === phoneId)?.number ?? coreT`your phone`;
}

type Dispatch = (a: Parameters<typeof signInReducer>[1]) => void;

/** Asks Apple to text or call phoneId and moves the form to its code. */
async function sendCode(dispatch: Dispatch, phoneId: number, mode: DeliveryMode) {
  dispatch({ type: "send-to-phone", phoneId, mode });
  try {
    await AuthService.SendCodeToPhone(phoneId, mode);
    dispatch({ type: "sent", target: { phoneId, mode } });
  } catch (err) {
    dispatch({ type: "failed", error: classifySignInError(err) });
  }
}

const methods = [
  { method: "device", label: () => coreT`Apple device`, Icon: MonitorSmartphone },
  { method: "sms", label: () => coreT`Text message`, Icon: MessageSquareText },
  { method: "voice", label: () => coreT`Phone call`, Icon: Phone },
] as const;

/**
 * Every way to get the code, always shown. The Apple device option is
 * disabled for accounts without a trusted device; text and call for
 * accounts without a trusted number.
 */
function MethodSwitcher({
  factor,
  current,
  busy,
  onPick,
}: {
  factor: SecondFactor;
  current: CodeMethod | undefined;
  busy: boolean;
  onPick: (m: CodeMethod) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <fieldset
        className="flex items-center gap-1 self-start rounded-md bg-hover p-1"
        disabled={busy}
      >
        <legend className="sr-only">
          <Trans>How to get the code</Trans>
        </legend>
        {methods.map(({ method, label, Icon }) => (
          <label
            key={method}
            className="flex h-7 cursor-pointer items-center gap-1.5 rounded-sm px-3 text-sub text-ink-muted has-checked:bg-raised has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-ring has-disabled:cursor-not-allowed has-disabled:opacity-45"
          >
            <input
              type="radio"
              name="code-method"
              value={method}
              checked={current === method}
              disabled={method === "device" ? !factor.trustedDevices : factor.phones.length === 0}
              onChange={() => onPick(method)}
              className="sr-only"
            />
            <Icon aria-hidden className="size-3.5" />
            {label()}
          </label>
        ))}
      </fieldset>
      {!factor.trustedDevices && (
        <p className="text-meta text-ink-muted">
          <Trans>
            No Apple device is signed in to this account, so Apple sends codes by text or call.
          </Trans>
        </p>
      )}
    </div>
  );
}

function CodeForm({
  step,
  dispatch,
}: {
  step: Extract<SignInStep, { kind: "code" }>;
  dispatch: Dispatch;
}) {
  const { t } = useLingui();
  const [code, setCode] = useState("");
  const len = step.factor.codeLength || 6;
  const { phoneId, mode } = step.target;
  const phones = step.factor.phones;

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
  const pick = (m: CodeMethod) => {
    if (m === "device") dispatch({ type: "use-device" });
    // One number: text or call it now. Several: choose which.
    else if (phones.length === 1) void sendCode(dispatch, phones[0].id, m);
    else dispatch({ type: "choose-phone", mode: m });
  };
  const cancel = () => {
    void AuthService.CancelSignIn();
    dispatch({ type: "cancel" });
  };

  const phone = phoneLabel(step.factor, phoneId);
  const where =
    phoneId === 0
      ? t`Apple sent a ${len}-digit code to the devices signed in to this Apple Account. Enter it here to finish signing in.`
      : mode === "voice"
        ? t`Apple is calling ${phone} with a ${len}-digit code.`
        : t`Apple sent a ${len}-digit code to ${phone} by text message.`;

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
        <FlowTitle>
          <Trans>Enter the verification code</Trans>
        </FlowTitle>
        <p className="mt-1 text-ui text-ink-muted">{where}</p>
      </div>
      <MethodSwitcher
        factor={step.factor}
        current={currentMethod(step)}
        busy={step.busy}
        onPick={pick}
      />
      {step.error && <SignInErrorNotice error={step.error} onBrowser={() => {}} />}
      {step.notice && !step.error && (
        <p role="status" className="text-sub text-ink">
          {step.notice}
        </p>
      )}
      <Field
        id="security-code"
        label={t`Code`}
        hint={t`${plural(len, { one: "# digit", other: "# digits" })}`}
      >
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
          className={cn(INPUT_CLS, "w-48 text-center text-heading tracking-[0.4em] num")}
          style={{ width: `${len * 1.6 + 2}em` }}
        />
      </Field>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={step.busy || code.length !== len}>
          {step.busy && <LoaderCircle className="animate-spin" />}
          {step.busy ? <Trans>Checking…</Trans> : <Trans>Verify</Trans>}
        </Button>
        <Button type="button" variant="ghost" disabled={step.busy} onClick={() => void resend()}>
          {phoneId === 0 ? (
            <Trans>Send a new code</Trans>
          ) : mode === "voice" ? (
            <Trans>Call again</Trans>
          ) : (
            <Trans>Text again</Trans>
          )}
        </Button>
        {phoneId !== 0 && phones.length > 1 && (
          <Button
            type="button"
            variant="ghost"
            disabled={step.busy}
            onClick={() => dispatch({ type: "choose-phone" })}
          >
            <Trans>Use another number</Trans>
          </Button>
        )}
        <Button type="button" variant="ghost" className="ml-auto" onClick={cancel}>
          <Trans>Cancel</Trans>
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
  dispatch: Dispatch;
}) {
  const phones = step.factor.phones;
  const [phoneId, setPhoneId] = useState(phones[0]?.id ?? 0);
  const { mode } = step;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <FlowTitle>
          {mode === "voice" ? (
            <Trans>Which number should Apple call?</Trans>
          ) : (
            <Trans>Which number should Apple text?</Trans>
          )}
        </FlowTitle>
        <p className="mt-1 text-ui text-ink-muted">
          <Trans>Choose one of the trusted phone numbers on this Apple Account.</Trans>
        </p>
      </div>
      <MethodSwitcher
        factor={step.factor}
        current={mode}
        busy={step.busy}
        onPick={(m) =>
          dispatch(m === "device" ? { type: "use-device" } : { type: "choose-phone", mode: m })
        }
      />
      {step.error && <SignInErrorNotice error={step.error} onBrowser={() => {}} />}
      <fieldset className="flex flex-col gap-1">
        <legend className="sr-only">
          <Trans>Trusted phone number</Trans>
        </legend>
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
      <div className="flex items-center gap-2">
        <Button
          onClick={() => void sendCode(dispatch, phoneId, mode)}
          disabled={step.busy || !phoneId}
        >
          {step.busy && <LoaderCircle className="animate-spin" />}
          {step.busy ? (
            <Trans>Sending…</Trans>
          ) : mode === "voice" ? (
            <Trans>Call me</Trans>
          ) : (
            <Trans>Text me</Trans>
          )}
        </Button>
        <Button
          variant="ghost"
          onClick={() => {
            void AuthService.CancelSignIn();
            dispatch({ type: "cancel" });
          }}
        >
          <Trans>Cancel</Trans>
        </Button>
      </div>
    </div>
  );
}

/** The browser fallback while Apple's page is open in the system browser. */
function BrowserWaiting({ status }: { status: AuthStatus }) {
  const { t } = useLingui();
  // Every sign-in link works once, so each button asks Sona for a new one.
  const withLink = async (act: (url: string) => Promise<unknown>) => {
    let url: string;
    try {
      url = await AuthService.SignInLink();
    } catch (err) {
      toast.error(t`Couldn't get a sign-in link`, { description: callFailure(err).message });
      return;
    }
    await act(url);
  };
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <LoaderCircle
          aria-hidden
          className="size-5 animate-spin text-ink-muted"
          strokeWidth={1.75}
        />
        <div>
          <FlowTitle>
            <Trans>Finish signing in in your browser</Trans>
          </FlowTitle>
          <p className="mt-0.5 text-ui text-ink-muted">
            <Trans>
              Apple's sign-in page is open in your browser. Sona continues by itself when you're
              done.
            </Trans>
          </p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {status.browserSignIn && (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void withLink((url) => Browser.OpenURL(url))}
            >
              <Trans>Open the page again</Trans>
              <ExternalLink strokeWidth={1.75} />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() =>
                void withLink((url) =>
                  Clipboard.SetText(url).then(() => toast(t`Sign-in link copied`)),
                )
              }
            >
              <Trans>Copy link</Trans>
            </Button>
          </>
        )}
        <Button variant="ghost" size="sm" onClick={() => void AuthService.CancelSignIn()}>
          <Trans>Cancel</Trans>
        </Button>
      </div>
    </div>
  );
}

/** Manual token paste: the last-resort fallback for developers and odd accounts. */
function TokenPaste() {
  const { t } = useLingui();
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
          ? t`Apple didn't accept that token. Copy a fresh one from a signed-in Apple Music web session.`
          : code === "network"
            ? t`Couldn't reach Apple to check the token. Check your connection and try again.`
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
        <Trans>Paste a Music User Token</Trans>
      </button>
    );
  }
  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-3">
      <Field
        id="mut"
        label={t`Music User Token`}
        hint={t`The token from a signed-in Apple Music web session. Sona checks it with Apple and keeps it in your keyring.`}
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
          {busy ? <Trans>Checking…</Trans> : <Trans>Use this token</Trans>}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setOpen(false)}>
          <Trans>Cancel</Trans>
        </Button>
      </div>
    </form>
  );
}

function SignInFlow({ status, actions }: { status: AuthStatus; actions?: HTMLElement | null }) {
  const [step, dispatch] = useReducer(signInReducer, initialSignIn);

  // Keep the form in step with Go: restored challenges, timeouts, sign-in elsewhere.
  useEffect(() => {
    dispatch({
      type: "status",
      state: status.state,
      factor: status.secondFactor,
      lastErrorCode: status.lastErrorCode,
      lastAppleMessage: status.lastAppleMessage,
    });
  }, [status.state, status.secondFactor, status.lastErrorCode, status.lastAppleMessage]);

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
    <div>
      {browserPending ? (
        <BrowserWaiting status={status} />
      ) : step.kind === "code" ? (
        <CodeForm step={step} dispatch={dispatch} />
      ) : step.kind === "choose-phone" ? (
        <PhoneChoice step={step} dispatch={dispatch} />
      ) : step.kind === "done" ? (
        <p className="flex items-center gap-2 text-ui text-ink-muted">
          <LoaderCircle aria-hidden className="size-4 animate-spin" />
          <Trans>Checking your account with Apple…</Trans>
        </p>
      ) : (
        <>
          {/* A host with its own foot (the setup) titles the form itself. */}
          {!actions && (
            <div className="mb-5">
              <FlowTitle>
                <Trans>Sign in to Apple Music</Trans>
              </FlowTitle>
              <p className="mt-1 text-ui text-ink-muted">
                <Trans>Use the Apple Account with your Apple Music subscription.</Trans>
              </p>
            </div>
          )}
          <CredentialsForm
            step={step}
            dispatch={dispatch}
            onBrowser={() => void browser()}
            actions={actions}
          />
        </>
      )}
      {step.kind === "credentials" && !browserPending && (
        <div className="mt-6 flex flex-col items-start gap-3 border-t border-line pt-5">
          <p className="text-sub text-ink-muted">
            <Trans>Use a passkey, a security key or a browser you're already signed in to:</Trans>
          </p>
          <Button variant="outline" size="sm" onClick={() => void browser()} disabled={step.busy}>
            <Trans>Sign in with your browser</Trans>
            <ExternalLink strokeWidth={1.75} />
          </Button>
          <TokenPaste />
        </div>
      )}
    </div>
  );
}

/** "en-GB" as the interface's language names it ("British English"); the tag when Intl can't. */
function languageName(tag: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(tag) ?? tag;
  } catch {
    return tag;
  }
}

function SignedIn({ status }: { status: AuthStatus }) {
  const { t, i18n } = useLingui();
  const [confirm, setConfirm] = useState(false);
  const [checking, setChecking] = useState(false);

  const revalidate = async () => {
    setChecking(true);
    try {
      await AuthService.Revalidate();
    } catch (err) {
      toast.error(t`Couldn't reach Apple`, { description: callFailure(err).message });
    } finally {
      setChecking(false);
    }
  };

  const store = status.storefrontName || status.storefrontId.toUpperCase();
  const language = status.language && languageName(status.language, i18n.locale);
  return (
    <div className="flex flex-col gap-4">
      {status.offline && (
        <Notice
          icon={<WifiOff aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />}
          action={
            <Button variant="ghost" size="sm" onClick={() => void revalidate()} disabled={checking}>
              {checking ? <Trans>Checking…</Trans> : <Trans>Check now</Trans>}
            </Button>
          }
        >
          <Trans>
            Sona couldn't reach Apple to confirm your sign-in. It keeps using your saved token, and
            retries by itself.
          </Trans>
        </Notice>
      )}
      <div className="divide-y divide-line border-y border-line">
        <div className="flex items-center justify-between gap-6 py-4">
          <div>
            <p className="text-ui font-medium text-ink">
              {status.keyring === "waiting" ? (
                <Trans>Saving your sign-in in your keyring; unlock it if it asks.</Trans>
              ) : status.state === "validating" ? (
                <Trans>Checking your sign-in…</Trans>
              ) : (
                <Trans>Signed in to Apple Music</Trans>
              )}
            </p>
            <p className="mt-0.5 text-sub text-ink-muted">
              {store ? (
                <Trans>Storefront: {store}</Trans>
              ) : (
                <Trans>Storefront unknown until Apple confirms the token</Trans>
              )}
              {language && ` · ${language}`}
            </p>
          </div>
          <Button variant="destructive" size="sm" onClick={() => setConfirm(true)}>
            <Trans>Sign out</Trans>
          </Button>
        </div>
        {status.restricted && (
          <div className="py-4 text-sub text-ink-muted">
            <Trans>Explicit content is restricted on this account; Sona hides it.</Trans>
          </div>
        )}
        {/* An unencrypted file gets its own notice below, which says where the token is. */}
        {!status.unencrypted && (
          <div className="flex items-start justify-between gap-6 py-4">
            <div>
              <p className="text-ui font-medium text-ink">
                <Trans>Where your token is kept</Trans>
              </p>
              <p className="mt-0.5 text-sub text-ink-muted">{status.backend || t`Unknown`}</p>
            </div>
          </div>
        )}
      </div>
      {status.unencrypted && (
        <Notice
          tone="problem"
          icon={<ShieldAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
        >
          <p className="font-medium">
            <Trans>Your token is stored unencrypted</Trans>
          </p>
          <p className="mt-0.5 text-ink-muted">
            <Trans>
              No Secret Service keyring is running, so Sona keeps your Apple Music token in a file
              only your user can read. Start GNOME Keyring or KWallet and Sona moves it into the
              keyring.
            </Trans>
          </p>
        </Notice>
      )}
      <SignOutDialog open={confirm} onOpenChange={setConfirm} />
    </div>
  );
}

/**
 * Asks before signing out of Apple Music (the account dialog's Sign out
 * button and the account menu's item), then signs out; a failure stays
 * open with a toast.
 */
export function SignOutDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLingui();
  const [busy, setBusy] = useState(false);
  const signOut = async () => {
    setBusy(true);
    try {
      await AuthService.SignOut();
      onOpenChange(false);
    } catch (err) {
      toast.error(t`Couldn't sign out`, { description: callFailure(err).message });
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Sign out of Apple Music?</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              Sona removes your token from the keyring, stops playback and deletes the copy of your
              library on this computer. Your Apple Music library itself isn't changed.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            <Trans>Cancel</Trans>
          </Button>
          <Button variant="destructive" onClick={() => void signOut()} disabled={busy}>
            {busy ? <Trans>Signing out…</Trans> : <Trans>Sign out</Trans>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Where the keyring holds sign-in up: the stored sign-in still being read,
 * a keyring that didn't answer or failed, or no keyring at all (a sign-in
 * would go to the unencrypted file). The retry keeps its place across
 * these, so focus stays on it while it reads again.
 */
function KeyringNotice({ status }: { status: AuthStatus }) {
  const restoring = status.state === "restoring";
  if (
    restoring
      ? status.keyring !== "waiting"
      : status.keyring !== "unavailable" && !status.unencrypted
  ) {
    return null;
  }
  const action = (
    <KeyringRetry status={status} className={buttonVariants({ variant: "ghost", size: "sm" })} />
  );
  if (restoring) {
    return (
      <Notice
        icon={
          <LoaderCircle
            aria-hidden
            className="size-4 animate-spin text-ink-muted motion-reduce:animate-none"
            strokeWidth={1.75}
          />
        }
        action={action}
      >
        <p className="font-medium">
          <Trans>Signing in…</Trans>
        </p>
        <p className="mt-0.5 text-ink-muted">
          <Trans>
            Sona is waiting for your keyring to hand over your sign-in. If GNOME Keyring or KWallet
            asks you to unlock it, do that now.
          </Trans>
        </p>
      </Notice>
    );
  }
  return (
    <Notice
      tone="problem"
      live="status"
      icon={<ShieldAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
      action={action}
    >
      {status.keyring === "unavailable" ? (
        <>
          <p className="font-medium">
            <Trans>Sona couldn't read your sign-in from your keyring</Trans>
          </p>
          <p className="mt-0.5 text-ink-muted">
            <Trans>
              Your keyring didn't answer or refused, so you're signed out. Unlock GNOME Keyring or
              KWallet, then try again.
            </Trans>
          </p>
        </>
      ) : (
        <>
          <p className="font-medium">
            <Trans>Signing in now stores your token unencrypted</Trans>
          </p>
          <p className="mt-0.5 text-ink-muted">
            <Trans>
              Sona couldn't open a keyring, so it would keep your Apple Music token in a file only
              your user can read. Start or unlock GNOME Keyring or KWallet, then try again.
            </Trans>
          </p>
        </>
      )}
    </Notice>
  );
}

/**
 * Apple Music sign-in and the signed-in account: the account dialog's body,
 * also embedded in the first-run setup's Apple Music step. Its holder is
 * the frame, so nothing here draws a panel of its own. `actions` is an
 * element in a host's own foot (the setup's): the password form's Sign in
 * goes there, so it stays in view however far the form scrolls, and the
 * form's own heading is left to the host.
 */
export function AccountSection({ actions }: { actions?: HTMLElement | null } = {}) {
  const auth = useAuth();
  const { t } = useLingui();
  if (auth.error) {
    return (
      <ErrorBlock
        title={t`Couldn't read the sign-in state`}
        error={auth.error}
        onRetry={() => void auth.refetch()}
        retrying={auth.isRefetching}
      />
    );
  }
  const status = auth.data;
  if (!status) {
    return <div aria-busy className="h-48 rounded-lg bg-surface" />;
  }
  if (status.state === "signed_in" || status.state === "validating") {
    return <SignedIn status={status} />;
  }
  const restoring = status.state === "restoring";
  return (
    <div className="flex flex-col gap-4">
      <KeyringNotice status={status} />
      {restoring ? (
        status.keyring !== "waiting" && <div aria-busy className="h-48 rounded-lg bg-surface" />
      ) : (
        <>
          {status.state === "needs_reauth" && (
            <Notice
              icon={<CircleAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
            >
              <p className="font-medium">
                <Trans>Apple stopped accepting Sona's sign-in</Trans>
              </p>
              <p className="mt-0.5 text-ink-muted">
                <Trans>
                  This happens after a password change or when the token expires. Your library on
                  this computer is still here; sign in again to keep syncing and playing.
                </Trans>
              </p>
            </Notice>
          )}
          {status.state === "error" && (
            <Notice
              icon={<CircleAlert aria-hidden className="size-4 text-danger" strokeWidth={1.75} />}
            >
              <p className="font-medium">
                <Trans>Apple Music isn't accepting Sona's requests</Trans>
              </p>
              <p className="mt-0.5 text-ink-muted">
                <Trans>
                  Sona refetches its developer token by itself; if this persists, Apple changed its
                  web player.
                </Trans>
              </p>
            </Notice>
          )}
          {status.offline && (
            <Notice
              icon={<WifiOff aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />}
            >
              <Trans>You're offline. Signing in needs a connection to Apple.</Trans>
            </Notice>
          )}
          <SignInFlow status={status} actions={actions} />
        </>
      )}
    </div>
  );
}
