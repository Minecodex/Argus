type Load = {
  start(release: () => void): void;
  active: boolean;
  done: boolean;
};

/** Bound opaque Runtime boot requests across every card in a conversation. */
export function createTemplateLoadQueue(limit = 4) {
  const pending: Load[] = [];
  let active = 0;
  let draining = false;
  const drain = () => {
    if (draining) return;
    draining = true;
    try {
      while (active < limit && pending.length) {
        const load = pending.shift()!;
        if (load.done) continue;
        load.active = true;
        active++;
        load.start(() => finish(load));
      }
    } finally {
      draining = false;
    }
  };
  const finish = (load: Load) => {
    if (load.done) return;
    load.done = true;
    if (load.active) active--;
    else {
      const index = pending.indexOf(load);
      if (index >= 0) pending.splice(index, 1);
    }
    drain();
  };
  return (start: Load["start"]) => {
    const load = { start, active: false, done: false };
    pending.push(load);
    drain();
    return () => finish(load);
  };
}

export const scheduleTemplateLoad = createTemplateLoadQueue();
