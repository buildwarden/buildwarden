---
name: workflow-intake
description: Adds a feature to the workflow queue - checks archived designs for overlap, picks a slug, priority and budget estimate with Jeff, and writes queue/<feature>.yaml. Use when Jeff has a new feature idea, or a session finds a regression main already has and files it as the item's follow-up (attended or not).
---

# Feature intake

Adds one queue item (FLOW-03). Its Design phase happens later, in the loop;
intake only frames the item. Rules: FLOW-01, FLOW-05, FLOW-12, FLOW-13,
FLOW-20.

1. Set up as in `workflow-tick` step 1.
2. Search `docs/design/` front-matter (`concepts`, `drivers`, `seams`) and the
   queue for overlap (FLOW-01, FLOW-12). If an item already covers the idea,
   say so and stop, or extend that item with Jeff's agreement.
3. Agree with Jeff on the kind (FLOW-03): `process` when the change is to
   the rules the loops run under (`docs/development/`, `.claude/`,
   `tools/workflow/`) and no feature needs it, otherwise a feature; a
   feature that needs a rule changed declares it in its design (FLOW-14).
   A process item also lists the
   workers it holds while in progress (`cloud`, `host:*`, `host:<name>`), or
   none when it disrupts no running work (FLOW-05). While FLOW-20 stands,
   agree too whether the item is a program item; one that is not is filed as
   FLOW-20's freeze says, unless it is exempt there. Then agree on
   a kebab-case slug, a one-line title, the priority (show
   `dist/wf status`, which lists FLOW-05's order: the class order decides
   where an item can go, priority places it within its class, and a new item
   never jumps an in-progress item of its own class without his say), and an
   estimate in steps and runs. A fix or follow-up of another item sets
   `-follows <item>`: it is then a fix (FLOW-05 class 3) and starts only once
   that item is done.
   Size guide: small 7 to 9 steps, one run; medium 12 to 18; large 25 or more.
   The runaway check escalates at `runaway_factor` (2) times the step
   estimate, while the item is open (FLOW-13).
4. Write it:

   ```bash
   dist/wf new -state .dev/workflow -worker human/jeff -feature <slug> \
     -title "<title>" -priority <n> -estimate-steps <n> -estimate-runs <n> \
     -source "<where it came from>" [-follows <item>] \
     [-kind process -holds cloud,host:*]
   dist/wf validate -state .dev/workflow
   ```

   Steps that need a native host for design or implementation get their
   affinity changed to `host:<name>` in the new file (FLOW-09).
5. Commit and push the workflow branch.

## A regression's follow-up

A session that finds a regression `main` already has, while an item is
built, validated or consolidated, files it at once as that item's follow-up
(FLOW-09), attended or not; an unattended session files it without Jeff. A
failure the item's own branch causes is not a follow-up: it goes back to the
item with `wf back` (FLOW-06). Steps 1, 2 and 4 apply, with these defaults in
place of step 3's agreement:

- a feature, `-follows <item>`, a small estimate (7 to 9 steps, one run), and
  the evidence in `-source` (the failing command or case, and where on `main`
  it also fails); logs go under `native/` as today;
- `-priority`: the top of the fixes. That is the lowest number held by an
  open class-3 item (a feature with `follows` set, not yet in
  implementation); with none, the lowest held by an open class-4 item; with
  neither, one past the highest open number. `wf new` renumbers the rest and logs each move.
  Several found together are queued in the order found, each at the number
  after the last.

Report each one as "<slug> (follows <item>)" in the step's evidence and in
the run's **Follow-ups filed** line. Jeff's explicit word overrides the
default and is recorded in the queue history: he may fold the fix into the
item (`wf back` with the finding), place it elsewhere (clear `follows`, or set
its priority), or leave it unfiled.
