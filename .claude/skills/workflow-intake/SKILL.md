---
name: workflow-intake
description: Adds a feature to the workflow queue - checks archived designs for overlap, picks a slug, priority and budget estimate with Jeff, and writes queue/<feature>.yaml. Use when Jeff has a new feature idea, or a native session finds a bug that needs its own item.
---

# Feature intake

Adds one queue item (FLOW-03). Its Design phase happens later, in the loop;
intake only frames the item. Rules: FLOW-01, FLOW-05, FLOW-12, FLOW-13.

1. Set up as in `workflow-tick` step 1.
2. Search `docs/design/` front-matter (`concepts`, `drivers`, `seams`) and the
   queue for overlap (FLOW-01, FLOW-12). If an item already covers the idea,
   say so and stop, or extend that item with Jeff's agreement.
3. Agree with Jeff: a kebab-case slug, a one-line title, the priority (show
   `dist/wf status`; a new item never jumps the top-priority feature in
   progress without his say, FLOW-05), and an estimate in steps and runs.
   Size guide: small 7 to 9 steps, one run; medium 12 to 18; large 25 or more.
   The runaway check escalates at twice the step estimate (FLOW-13).
4. Write it:

   ```bash
   dist/wf new -state .dev/workflow -worker human/jeff -feature <slug> \
     -title "<title>" -priority <n> -estimate-steps <n> -estimate-runs <n> \
     -source "<where it came from>"
   dist/wf validate -state .dev/workflow
   ```

   Steps that need a native host for design or implementation get their
   affinity changed to `host:<name>` in the new file (FLOW-09).
5. Commit and push the workflow branch.
