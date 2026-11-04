---
feature: human-review-shows-design
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-07, FLOW-14, FLOW-19]
concepts: [workflow, design-review, workflow-human, assumptions]
related: []
---

# Design reviews put the design and its assumptions in front of Jeff

## Problem

In the build-link-subnet design review r1 (workflow-human, 2026-11-03), the
assumptions were asked by their IDs and one-line summaries. The design
document that names them, with their options, reasons and the decisions they
rest on, was on the feature branch in the cloud checkout. Jeff did not have it
locally, so he had to answer from a summary or go and find the file.

`workflow-human` step 3 says to "walk Jeff through" the design. It does not
say the design itself has to reach him. Step 3.4 says only that assumptions
are "confirmed with `dist/wf answer`". That reads as if the questions can be
asked bare.

## Prior art

- FLOW-19 makes the design review the point where Jeff confirms assumptions
  and approves the design and plan. A review he cannot read is not one.
- Step 3.1 already allows "an artifact page when a diagram makes it clearer",
  but only for the overview.
- FLOW-07 asks for a question with options and a recommended answer. It does
  not say where the question's context comes from.

## Decisions

1. **Put the design in front of Jeff for every review, by size.** At the
   start of each design review:
   - **More than 4 assumed questions:** publish the design as it is read
     (front-matter, Decisions, Questions, Security note, Validation plan,
     Implementation plan) as a private artifact page, and give Jeff the link
     before the walkthrough.
   - **4 or fewer:** paste the design's Questions section in full into the
     chat, with a short summary of the Decisions they rest on.

   Four is what one AskUserQuestion call holds with its context inline
   (Jeff, review r1).
   - **Rejected: a page for every review.** It costs a publish for small
     designs that fit in the chat.
   - **Rejected: inline only.** It does not scale to build-link-subnet's ten
     assumptions and 39 tasks, and it leaves no page to come back to.
   - **Rejected: link to the file on GitHub.** That needs Jeff to be on a
     machine with a browser and repo access, and it shows raw Markdown, not
     the rendered decisions next to their questions.
   - **Fallback:** if publishing is not available, put the full Questions
     section inline in the chat before asking, whatever its size.
2. **Quote each assumption when asking it.** Each AskUserQuestion for an
   assumption carries:
   - the question in full, not an ID;
   - each option with its consequence;
   - the recommendation first, marked "(Recommended)";
   - the decision it rests on, named.

   One question per assumption, at most four per call.
3. **The page is read-only and per sitting.** It is published from the
   design file at the commit being reviewed, with the commit SHA in its
   subtitle. It is republished if Jeff changes the design during the sitting.
   The review itself stays in the session, and decisions are recorded in the
   design's Design review section and with `dist/wf answer`.

## Questions

### Blocking

None.

### Assumed

- **a1: answered (c), by Jeff in r1.** Publish a page above a threshold of
  more than 4 assumptions, and inline at or below it. Options were: (a) a
  page for every review (recommended at the time); (b) the inline Questions
  section only; (c) a page above a threshold.
- **a2: confirmed (a), by Jeff in r1.** The change is to
  `.claude/skills/workflow-human/SKILL.md` step 3 only. No rule in `docs/development/` changes. Options: (a) skill only
  (recommended, since FLOW-19 already requires a real review); (b) also add a
  sentence to FLOW-19 that the reviewer must have the design in front of
  them.

## Design

`.claude/skills/workflow-human/SKILL.md`, step 3 (Design reviews):

- After "read the design, its Implementation plan, and ...", add: "Put the
  design in front of Jeff before the walkthrough. If it has more than 4
  assumed questions, publish it as read as a private artifact page (the
  Artifact tool), with the commit SHA in its subtitle, give him the link, and
  republish if he changes the design during the sitting. Otherwise, or if
  publishing is not available, paste its Questions section in full with a
  short summary of the Decisions they rest on."
- In 3.1, the existing "render it as an artifact page when a diagram makes it
  clearer" becomes part of that page.
- Replace 3.4 with: "**Assumptions**, one per AskUserQuestion entry: the
  question in full, each option with its consequence, the recommendation
  first, and the decision it rests on. Record each answer with `dist/wf
  answer`, or turn it into a change."

## Cross-driver impact

None. No driver or product code changes.

## Validation plan

- **Tier 0:** none. This is a skill text change with no code.
- **Check:** the next design review that runs with the new skill (expected
  build-link-subnet r2 or later) publishes a page and asks each assumption in
  full. The Design review entry for that sitting records the page link.

## Implementation plan

1. Edit `.claude/skills/workflow-human/SKILL.md` step 3 as in Design (FLOW-14,
   FLOW-19, FLOW-07; a1, a2).
2. Archive this design with its summary and open the PR (FLOW-12, FLOW-08).

## Design review

- **2026-11-04, r1: approved** (workflow-human, Jeff), approved at 87466b4,
  with one change folded in.
  - a1 answered (c): a page only above more than 4 assumptions, the inline
    Questions section otherwise. Decision 1 and Design were updated to match.
  - a2 confirmed (a): skill only, no FLOW-19 change.
  - This review was itself held inline: 2 assumptions, below the new
    threshold.

## Consolidation

## Rollback notes

## Archive summary
