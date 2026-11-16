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
   A change to a loop rule (LOOP-\*, FLOW-\*, LINT-\*) or to its enforcement
   is a process item, never a feature's rule change (LOOP-09). Agree with
   Jeff which repositories the item works in, its home first (`-repos`,
   when there is more than this one, LOOP-04), and which areas it changes,
   by the names `dist/wf areas -state .dev/workflow -list` prints
   (`-areas`; FLOW-02, FLOW-05); `wf areas -list` and `wf areas -check` exit 1 while the queue state has no `areas.yaml`, which is expected until `loop-parallel`'s `x4-state` writes it: say so and go on. Leaving `-areas` out files the item
   undeclared, colliding with every item in its repositories until its
   design declares them. A feature whose home is a repository where the
   loop's gates do not run is tracked (FLOW-03): give it `-repos` with that
   repository first and the seams it changes with `-seams`. A
   reconciliation item (LOOP-07) sets `-adopts <consumable>@<version>`: it
   is in class 3 and goes below the open fixes unless Jeff says otherwise
   (FLOW-05).
   A process item also lists the
   workers it holds while in progress (`cloud`, `host:*`, `host:<name>`), or
   none when it disrupts no running work (FLOW-05). While FLOW-20 stands,
   agree too whether the item is a program item; one that is not is filed as
   FLOW-20's freeze says, with `-hold "<why>"`, unless it is exempt there.
   Then agree on
   a kebab-case slug, a one-line title, the priority (show
   `dist/wf status`, which lists FLOW-05's order: the class order decides
   where an item can go, priority places it within its class, and a new item
   never jumps an in-progress item of its own class without his say), and an
   estimate in steps and runs. A fix or follow-up of another item sets
   `-follows <item>`: it is then a fix (FLOW-05 class 3) and starts only once
   that item is done.
   When the waiting limit is reached (`dist/wf next` prints it), say so: a
   new feature will not start until an item clears; a fix with `-follows`
   will, and so will a security fix Jeff marks urgent; and the number is
   Jeff's to change (FLOW-05).
   Judge whether the item is a security fix, as you judge FLOW-20's
   exemption for one. When it is one and follows no item, offer `-urgent`,
   whatever the count is that day, and pass it only on Jeff's word. Do not
   offer it for anything else, and decline it when asked for anything
   else, saying that the mark is for a security fix and that the limit is
   his to raise (FLOW-05, FLOW-20).
   Size guide: small 7 to 9 steps, one run; medium 12 to 18; large 25 or more.
   The runaway check escalates at `runaway_factor` (2) times the step
   estimate, while the item is open (FLOW-13).
4. Write it:

   ```bash
   dist/wf new -state .dev/workflow -worker human/<name> -feature <slug> \
     -title "<title>" -priority <n> -estimate-steps <n> -estimate-runs <n> \
     -source "<where it came from>" [-follows <item>] \
     [-repos <home>,<other>] [-areas <a>,<b>] [-seams <s>] \
     [-adopts <consumable>@<version>] [-urgent] \
     [-kind process -holds cloud,host:*] [-hold "<why it is held>"]
   dist/wf validate -state .dev/workflow
   dist/wf relations -state .dev/workflow -feature <slug>
   ```

   `<name>` is the human at the intake. Show Jeff the relations `wf
   relations` prints: the open items the new one is near or collides with,
   and why (FLOW-05).

   `-hold` files the item with an open blocking question `q1` that only Jeff
   answers (FLOW-20, FLOW-07); `dist/wf answer -question q1` resumes it.
   After a `-hold`, `dist/wf status` must show the item's
   `blocking question q1`, and after `-urgent` its `urgent:` line: `wf` stops reading flags at the first word that
   is not one, such as the second word of an unquoted value, and says
   nothing, so a hold or a mark after it is dropped. If the line is missing, do not
   commit (step 5), since the loop would be offered the item at once:
   discard the uncommitted queue change and write the item again.

   Steps that need a native host for design or implementation get their
   affinity changed to `host:<name>` in the new file (FLOW-09).
5. Push it with the same worker as step 4:
   `dist/wf commit -state .dev/workflow -worker <worker> -m "<slug>: intake" queue/`
   (`queue/`, since `wf new` may renumber other items; `workflow-tick` step 2
   gives its exit codes).

## A regression's follow-up

A session that finds a regression `main` already has, while an item is
built, validated or consolidated, files it at once as that item's follow-up
(FLOW-09), attended or not; an unattended session files it without Jeff. So
does a run that sees CI fail on `main` after a merge: the fix follows the
item whose PR that merge was. A failure the item's own branch causes is not a
follow-up: it goes back to the item with `wf back` (FLOW-06). Steps 1, 2 and 4
apply, with these defaults in place of step 3's agreement:

- `-worker $W`, the filing session's own worker (the tick's `cloud/<session>`,
  a native session's `host:<name>/<session>`) in place of step 4's, so that an
  item nobody was there to file is not filed under Jeff's name (FLOW-09);
- no `-urgent`, which `wf new` refuses with `-follows`, and no `-repos` or
  `-areas`: `wf new -follows` takes the followed item's repositories and
  areas, for the fix's design to confirm (FLOW-09);

- a feature, `-follows <item>`, a small estimate (7 to 9 steps, one run), and
  the evidence in `-source` (the failing command or case, and where on `main`
  it also fails); logs go under `native/` as today;
- `-priority`: the top of the fixes. That is the lowest number held by an
  open class-3 item (a feature with `follows` set, not yet in
  implementation); with none, the lowest held by an open class-4 item; with
  neither, one past the highest open number. `wf new` renumbers the rest and logs each move.
  Several found together are queued in the order found, each at the number
  after the last.

A fix for a red `main` is recorded with
`dist/wf main -state .dev/workflow -followup <slug>`; so is one Jeff files
for a red commit that was no item's (FLOW-05, FLOW-09), and `main.yaml` is
then among the paths step 5 passes to `wf commit`. Report each one as
"<slug> (follows <item>)" in the step's evidence and in the run's
**Follow-ups filed** line. Jeff's explicit word overrides the
default and is recorded in the queue history: he may fold the fix into the
item (`wf back` with the finding), place it elsewhere (clear `follows`, or set
its priority), or leave it unfiled.
