// The To, Cc and Bcc fields of the compose window: address tokens, a text
// input and suggestions (docs/specs/compose-ui.md, Recipient field and
// Behavior: Recipients). Task T-0043 implements it; the stub only shows the
// input.
import type { Address } from "../../rpc/gen/api";

/** RecipientFieldProps are a recipient field's inputs. */
export interface RecipientFieldProps {
  /** The field's name and the input's accessible name: "To", "Cc" or "Bcc". */
  label: string;
  /** The field's addresses, in order. */
  value: Address[];
  /** Called with the new list whenever tokens are added or removed. */
  onChange: (next: Address[]) => void;
  /** Suggestions for typed text; the container asks maild. */
  suggest: (prefix: string) => Promise<Address[]>;
  autoFocus?: boolean;
}

/** RecipientField is a token field for addresses. */
export function RecipientField(props: RecipientFieldProps) {
  return <input aria-label={props.label} />;
}
