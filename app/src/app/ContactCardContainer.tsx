// Opens the contact card for an address in the reader (docs/specs/pim-ui.md).
import { useEffect, useState } from "react";
import { useClient } from "../data/session";
import { useUI } from "../data/stores";
import { type AddState, ContactPopover } from "../features/people/ContactPopover";
import type { Address, ContactCard } from "../rpc/gen/api";
import { composeTo } from "./compose";
import { revealMessage } from "./openMessage";

/** ContactCardContainerProps say which card is open. */
export interface ContactCardContainerProps {
  address: Address;
  at: { x: number; y: number };
  onClose: () => void;
}

/** ContactCardContainer loads a contact card and carries out its actions. */
export function ContactCardContainer({ address, at, onClose }: ContactCardContainerProps) {
  const client = useClient();
  const [card, setCard] = useState<ContactCard | null>(null);
  const [photo, setPhoto] = useState<string | undefined>();
  const [add, setAdd] = useState<AddState>("idle");
  const name = card?.name.trim() || address.name.trim();

  useEffect(() => {
    let stopped = false;
    void client.people
      .card({ email: address.address })
      .then((result) => {
        if (!stopped) setCard(result);
      })
      .catch((err: unknown) => console.warn("people card", err));
    return () => {
      stopped = true;
    };
  }, [client, address.address]);

  const person = card?.person;
  useEffect(() => {
    let stopped = false;
    setPhoto(undefined);
    if (person?.hasPhoto) {
      void client.people
        .photo({ id: person.id })
        .then((result) => {
          if (!stopped) setPhoto(`data:${result.contentType};base64,${result.data}`);
        })
        .catch((err: unknown) => console.warn("people photo", err));
    }
    return () => {
      stopped = true;
    };
  }, [client, person]);

  const addContact = async () => {
    if (add === "adding" || add === "added") return;
    setAdd("adding");
    try {
      await client.people.add({ email: address.address, ...(name ? { name } : {}) });
      const fresh = await client.people.card({ email: address.address });
      setCard(fresh);
      setAdd("added");
    } catch (err: unknown) {
      setAdd({ error: err instanceof Error ? err.message : String(err) });
    }
  };

  return (
    <ContactPopover
      address={address}
      card={card}
      photo={photo}
      at={at}
      now={new Date()}
      add={add}
      onClose={onClose}
      onCompose={() => {
        void composeTo(client, address.address, name).catch((err: unknown) => console.warn("compose", err));
        onClose();
      }}
      onAdd={() => void addContact()}
      onOpenPerson={(id) => {
        const ui = useUI.getState();
        ui.setModule("people");
        ui.setPeopleBook("all");
        ui.clearPeopleSearch();
        ui.selectPerson(id);
        onClose();
      }}
      onOpenMessage={(id) => {
        void revealMessage(client, id).catch((err: unknown) => console.warn("open message", err));
        onClose();
      }}
    />
  );
}
