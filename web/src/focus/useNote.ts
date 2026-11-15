// A brief note: a string that says itself, goes after NOTE_MS, and restarts
// its NOTE_MS when a newer note replaces it.
import { useEffect, useState } from "react";

// NOTE_MS is how long a note stays.
export const NOTE_MS = 4000;

export function useNote(): { note: string | undefined; say(text: string): void } {
  // Each say is a new object, so repeating a text restarts the timer.
  const [note, setNote] = useState<{ text: string }>();
  useEffect(() => {
    if (!note) return;
    const timer = setTimeout(() => setNote(undefined), NOTE_MS);
    return () => clearTimeout(timer);
  }, [note]);
  return { note: note?.text, say: (text) => setNote({ text }) };
}
