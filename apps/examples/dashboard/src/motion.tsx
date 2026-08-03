import { motion } from "framer-motion";
import type { ReactNode } from "react";

import { motionDurationMs } from "./theme";

const fadeTransition = { duration: motionDurationMs / 1000, ease: "easeOut" as const };

/**
 * Wraps one routed page's content in a quick fade+rise on mount — kept
 * short (theme.ts's motionDurationMs) so it reads as polish on a short-form
 * video, not as sluggishness. Used once at the top of every page component
 * rather than around <Routes>, so a page keeps its own scroll position
 * instead of remounting the whole route tree on every navigation.
 */
export function PageFade({ children }: { children: ReactNode }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={fadeTransition}
    >
      {children}
    </motion.div>
  );
}

/** Staggers the entrance of a small list of items (e.g. a workflow's steps) — each one fades/rises in slightly after the previous. */
export function StaggerList({ children }: { children: ReactNode }) {
  return (
    <motion.div initial="hidden" animate="visible" variants={{ visible: { transition: { staggerChildren: 0.04 } } }}>
      {children}
    </motion.div>
  );
}

export function StaggerItem({ children }: { children: ReactNode }) {
  return (
    <motion.div
      variants={{ hidden: { opacity: 0, y: 8 }, visible: { opacity: 1, y: 0 } }}
      transition={fadeTransition}
    >
      {children}
    </motion.div>
  );
}

/** A subtle, continuous pulse for a "running" state — stops the instant the caller re-renders without runningState (e.g. status settles). */
export function RunningPulse({ children }: { children: ReactNode }) {
  return (
    <motion.div
      animate={{ opacity: [1, 0.55, 1] }}
      transition={{ duration: 1.1, repeat: Infinity, ease: "easeInOut" }}
      style={{ display: "inline-flex" }}
    >
      {children}
    </motion.div>
  );
}

/** Cross-fades between two pieces of content in the same slot (e.g. a form → its live run view) without unmounting/remounting the surrounding layout. */
export function CrossFade({ children, id }: { children: ReactNode; id: string }) {
  return (
    <motion.div key={id} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={fadeTransition}>
      {children}
    </motion.div>
  );
}
