import { useEffect, useRef, useState } from "react";

/** A resumed page gets a new epoch so hidden results cannot reappear as fresh. */
export function useDocumentVisibility() {
  const [visible, setVisible] = useState(() => !document.hidden);
  const [epoch, setEpoch] = useState(0);
  const previous = useRef(visible);
  useEffect(() => {
    const update = () => {
      const next = !document.hidden;
      if (next && !previous.current) setEpoch((value) => value + 1);
      previous.current = next;
      setVisible(next);
    };
    document.addEventListener("visibilitychange", update);
    return () => document.removeEventListener("visibilitychange", update);
  }, []);
  return { visible, epoch };
}
