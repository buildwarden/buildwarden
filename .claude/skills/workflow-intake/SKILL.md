---
name: workflow-intake
description: Adds a feature to the workflow queue - checks archived designs for overlap, picks a slug, priority and budget estimate with Jeff, and writes queue/<feature>.yaml. Use when Jeff has a new feature idea, or a session finds a regression main already has and files it as the item's follow-up (attended or not).
---

# Feature intake

Adds one queue item (FLOW-03). Its Design phase happens later, in the loop,
unless the same-sitting lane below (step 6) is taken; intake only frames the
item. Rules: FLOW-01, FLOW-05, FLOW-12, FLOW-13,
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
   when there is more than this one, LOOP-04). Each must have an entry in
   `config.yaml`'s `repositories`: refuse one that has none, and say that an
   attended session adds it, with its trunk, on Jeff's word (`wf validate`
   fails an item naming it). Agree which areas it changes,
   by the names `dist/wf areas -state .dev/workflow -list` prints
   (`-areas`; FLOW-02, FLOW-05). Leaving `-areas` out files the item
   undeclared, colliding with every item in its repositories until its
   design declares them; an area of another repository is written
   `<owner>/<name>:<area>` (FLOW-02). A feature whose home has not adopted
   the contract (its `repositories` entry names no `profile`) is tracked
   (FLOW-03): give it `-repos` with that repository first and the seams it
   changes with `-seams`. A
   reconciliation item (LOOP-07) sets `-adopts <consumable>@<version>`: it
   is in class 3 and goes below the open fixes unless Jeff says otherwise
   (FLOW-05).
   A process item also lists the
   workers it holds while in progress (`cloud`, `host:*`, `host:<name>`), or
   none when it disrupts no running work (FLOW-05). While FLOW-20 stands,
   agree too whether the item is a program item (its design will list
   `componentization.md` in `related`): one is filed with `-program`, placed
   above every released item of its class that is not a program item, and
   declares its repositories and areas where the program's design gives
   them. An item that is neither a program item nor a process item is held,
   a security or regression fix included, unless Jeff releases it: file it
   with `-hold "<why>" -resumes "<when>"`, `program-end` unless Jeff names
   another point (FLOW-20). Never held: the fix for a failing `main` or
   program trunk, and a fix that `follows` a program item; a non-program
   follow-up of a held item takes that item's resume point. A process item
   filed while FLOW-20 stands carries a **Program payoff** section in its
   design; its Design is `cloud`, `wf-design` creates its branch (step 6's
   "create `feat/<slug>`" holds only once FLOW-20 is retired), and it takes
   the process test unless it touches a rule FLOW-20 reserves to Jeff.
   Then agree on
   a kebab-case slug, a one-line title, the position (show
   `dist/wf status`, which lists FLOW-05's order with each open item's
   position: the class order decides where an item can go, `wf new` places
   it as near the asked position as its class and status allow and prints
   where it went, and a new item never goes ahead of an in-progress item of
   its own class), and an estimate in steps and runs. A fix or follow-up of another item sets
   `-follows <item>`: it is then a fix (FLOW-05 class 3) and starts only once
   that item is done.
   When the waiting limit is reached (`dist/wf next` prints it), say so: a
   new feature will not start until an item clears; a fix with `-follows`
   will, and so will a security fix Jeff marks urgent; and the number is
   Jeff's to change (FLOW-05).
   Judge whether the item is a security fix. When it is one and follows no
   item, offer `-urgent`,
   whatever the count is that day, and pass it only on Jeff's word. Do not
   offer it for anything else, and decline it when asked for anything
   else, saying that the mark is for a security fix and that the limit is
   his to raise (FLOW-05). The mark does not release a hold (FLOW-20).
   Size guide: small 7 to 9 steps, one run; medium 12 to 18; large 25 or more;
   a process item 4 steps, one run, plus 1 step for each expected state step
   (`x4-state`).
   The runaway check escalates at `runaway_factor` (2) times the step
   estimate, while the item is open (FLOW-13).
4. Write it:

   ```bash
   dist/wf new -state .dev/workflow -worker human/<name> -feature <slug> \
     -title "<title>" -priority <n> -estimate-steps <n> -estimate-runs <n> \
     -source "<where it came from>" [-follows <item>] \
     [-repos <home>,<other>] [-areas <a>,<b>] [-seams <s>] \
     [-adopts <consumable>@<version>] [-urgent] [-lead host:<name>] \
     [-kind process -holds cloud,host:*] [-program] \
     [-hold "<why it is held>" -resumes "<when it resumes>"]
   dist/wf validate -state .dev/workflow
   dist/wf relations -state .dev/workflow -feature <slug>
   ```

   `<name>` is the human at the intake. Show Jeff the relations `wf
   relations` prints: the open items the new one is near or collides with,
   and why (FLOW-05).

   `-hold` files the item with an open blocking question `q1` that only Jeff
   answers, naming when it resumes (`-resumes`, which `-hold` needs); a held
   item counts toward no limit and is listed once under **Held** (FLOW-20,
   FLOW-07, FLOW-05). `dist/wf answer -question q1` resumes it. An item
   already queued or in progress is held with `dist/wf hold -feature F -why
   "<why>" -resumes "<when>"`.
   After a `-hold`, `dist/wf status` must show the item's
   `blocking question q1`, and after `-urgent` its `urgent:` line: `wf` stops reading flags at the first word that
   is not one, such as the second word of an unquoted value, and says
   nothing, so a hold or a mark after it is dropped. If the line is missing, do not
   commit (step 5), since the loop would be offered the item at once:
   discard the uncommitted queue change and write the item again.

   A feature whose design or implementation needs a native host from its
   first step is led by that host: `-lead host:<name>`, which gives its steps
   of Branch setup through Guide that host, and seeds each later phase so
   (FLOW-04, FLOW-09). Never on a process item, whose native work is done in
   an attended native session or tracked as native gaps, and never on a
   tracked item. `dist/wf status` then shows its `lead:` line.
5. Push it with the same worker as step 4:
   `dist/wf commit -state .dev/workflow -worker <worker> -m "<slug>: intake" queue/`
   (`queue/`, since `wf new` renumbers the open items; `workflow-tick` step 2
   gives its exit codes).
6. **The same-sitting lane**, for a trivial process item only. It is trivial
   when Jeff states the exact edit at intake; the change is text only
   (rules, skills, agents, the template), with no `tools/workflow/` code, no
   state step and no `holds`; its review fits the sitting's ask cap
   (`workflow-human` item 3); and FLOW-20 does not stand. While FLOW-20
   stands the lane is not offered: the item's Design is `cloud`, and its
   process test, or `x2-design-review` when it touches a rule FLOW-20
   reserves to Jeff, takes the lane's place; Jeff can drive its Design
   himself through `/workflow-tick attended <slug>`. Otherwise do not
   offer it. When it holds,
   offer to draft the design and review it now, and on Jeff's yes: create
   `feat/<slug>` from `main` and push it (a process item has no
   branch-setup step); draft the design there from the template (Problem,
   Decisions, Questions, Rule changes, Implementation plan, Design review);
   write its assumed questions to the queue file and push them with
   `dist/wf commit` (`wf answer` refuses a question the file lacks); then
   hold `workflow-human` item 3 on it in this session. Its asks count
   toward the sitting's cap. LOOP-09's owner review and owner merge, and
   every rule-change ask, stay as they are.

## The loop's own filings (FLOW-20)

While FLOW-20 stands the loop files, unattended, with its own worker
(`-worker $W`) and recorded with `dist/wf record`: the items of a phase row
of the program's design once the gate before its wave is met (`-program`,
placed by the phase order), a reconciliation item that adopts a
pre-release on a program trunk (`-adopts`, LOOP-07), the relations between
items their designs name, and `program-sync-<main sha>` for a program sync
whose conflict it cannot resolve (`-program -escalate "<the conflicting
paths and both sides' commits>"`, whose first step is the escalation).
Steps 1, 2 and 4 apply; Jeff may call any back (FLOW-20).

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
- `-priority`: the top of the fixes: the position of the first open class-3
  item (a feature with `follows` set, not yet in implementation); with none,
  of the first open class-4 item; with neither, one past the last open
  position. `wf new` renumbers the rest.
  Several found together are queued in the order found, each at the number
  after the last.

A fix for a red `main` is recorded with
`dist/wf main -state .dev/workflow -followup <slug>`; so is one Jeff files
for a red commit that was no item's (FLOW-05, FLOW-09), and `main.yaml` is
then among the paths step 5 passes to `wf commit`. Report each one as
"<slug> (follows <item>)" in the step's evidence and in the run's
**Follow-ups filed** line. Jeff's explicit word overrides the
default and is recorded in the queue history: he may fold the fix into the
item (`wf back` with the finding), place it elsewhere (clear `follows`, or move
it with `wf move`), or leave it unfiled.
