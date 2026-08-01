# ADR-0018 — Timeouts, annulation et commandes CLI restantes du moteur de workflows

## Statut
Accepté

## Contexte
L'ADR-0017 avait délibérément laissé de côté, dans la checklist de la phase 3, les timeouts par étape, l'annulation, et une partie des commandes CLI (`workflow list/validate/export`, `run list/logs/cancel`). Cette passe les complète, à l'exception des événements temps réel (SSE), traités séparément vu leur ampleur (nécessitent une première vraie surface d'API HTTP).

Deux questions structurantes se sont posées en cours d'implémentation, la seconde révélée par un bug réel détecté en écrivant les tests.

## Décision

**Timeout par étape** : `runs.ExecuteOptions.StepTimeout` (défaut 30s, configurable via `patchcord workflow run --step-timeout`) borne chaque appel d'action via un contexte dérivé du contexte d'exécution. Un dépassement de ce timeout est traité comme un **échec de l'étape** (`RunFailed`), pas comme une annulation du run — distinction faite explicitement en ne réagissant qu'à `context.Canceled` (jamais à `context.DeadlineExceeded`) pour décider qu'un run est passé en `RunCancelled`.

**Contexte de persistance découplé du contexte d'exécution** : toutes les écritures de bookkeeping du runner (création du run, transitions d'étapes, statut final) utilisent désormais un contexte dédié (`persistTimeout`, 10s, dérivé de `context.Background()`), jamais le contexte de l'appelant. Seul l'appel réel à l'action (`executor.ExecuteAction`) reste lié au contexte de l'appelant via un timeout dérivé. **Bug réel corrigé** : avant cette séparation, annuler le contexte de l'appelant (ex. Ctrl+C) pouvait faire échouer l'écriture finale marquant le run comme `cancelled`, laissant potentiellement la ligne bloquée indéfiniment au statut `running` en base.

**Cohérence des statuts sur annulation** : quand un run se termine `RunCancelled`, l'étape interrompue est marquée `StepCancelled` (pas `StepFailed`) et les étapes jamais démarrées `StepCancelled` (pas `StepSkipped`, réservé au cas où c'est un échec ordinaire d'une étape antérieure qui a stoppé le run).

**`patchcord run cancel <run-id>`** ne fait que marquer en base un run encore `queued` ou `running` comme `cancelled` — cohérent avec le modèle d'exécution synchrone mono-processus posé par l'ADR-0017 (`workflow run` s'exécute dans son propre processus, du début à la fin). Cette commande ne peut donc **pas** interrompre un run activement en cours ailleurs ; elle sert uniquement à nettoyer un run laissé bloqué par un processus qui a crashé. L'annulation « en direct » d'un run en cours se fait par Ctrl+C/SIGTERM sur le processus `workflow run` lui-même, déjà géré via `signal.NotifyContext`.

## Conséquences positives
- Une action bloquée ne bloque plus indéfiniment un run : bornée par un timeout par étape raisonnable, configurable.
- Le bug de persistance découvert par les tests (statut final non enregistrable après annulation) est corrigé avant d'avoir jamais atteint un utilisateur — exactement le genre de défaut que l'exigence de tests explicites de CLAUDE.md est censée intercepter.
- La distinction failed/cancelled au niveau de l'étape comme du run donne une trace exacte de *pourquoi* un run s'est arrêté, utile pour `run logs`/`run inspect`.
- L'ensemble des commandes CLI prévues par la checklist phase 3 (hors SSE) est maintenant disponible : `workflow list/validate/export/run`, `run list/inspect/logs/cancel`.

## Conséquences négatives
- `run cancel` a une portée plus restreinte que ce que son nom suggère intuitivement — un utilisateur pourrait s'attendre à interrompre un run en cours depuis un autre terminal, ce qui n'est pas possible avec l'architecture actuelle. Documenté explicitement dans l'aide de la commande, mais reste une source de confusion potentielle tant qu'aucune vraie exécution asynchrone/API n'existe.
- Le timeout par étape est unique pour tout le workflow (`--step-timeout` global), pas configurable par étape individuelle — une action rapide et une action lente partagent la même borne par défaut, à affiner quand le modèle d'action complet (section 7.4) définira un timeout par défaut par action.
- Le contexte de persistance dédié (10s) est une borne arbitraire raisonnable mais non configurable ; une base SQLite anormalement lente ou verrouillée pourrait encore faire échouer l'enregistrement du statut final au-delà de cette fenêtre.
