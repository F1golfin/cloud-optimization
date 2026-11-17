# Test sur des VM jamais mesurées

Référence : cloudopt-vm2 (seule VM benchmarkée pour A et B). Exécutions : results/exp_newvms.
Colonnes A à E : médiane de (fin mesurée / fin prédite) de la VM, et entre parenthèses l'erreur moyenne.
Vitesse : par tâche, relative à la référence (> 1 = plus rapide). Contention réelle : ralentissement
d'une tâche quand tous les workers tournent (le modèle suppose 1,40).

| VM | Type | Exéc. | Vitesse réelle | Vitesse 1 tâche | Contention réelle | A fiche seule | B fiche + 1 tâche | D fiche + 4 tâches | E D + hyperthreading | C benchmark complet |
|---|---|---|---|---|---|---|---|---|---|---|
| cloudopt-n-e2std4 | e2-standard-4 | 6 | 1.06 | 1.03 | 1.42 | 0.92 (±8 %) | 0.94 (±6 %) | 0.96 (±4 %) | 0.96 (±4 %) | 0.95 (±5 %) |
| cloudopt-n-n2 | n2-standard-2 | 6 | 1.31 | 1.29 | 1.36 | 0.73 (±27 %) | 0.90 (±10 %) | 0.92 (±8 %) | 0.92 (±8 %) | 0.91 (±9 %) |
| cloudopt-n-n2d | n2d-standard-2 | 6 | 2.03 | 4.66 | 1.28 | 0.60 (±40 %) | 1.83 (±83 %) | 0.90 (±10 %) | 0.90 (±10 %) | 0.90 (±10 %) |
| cloudopt-n-t2d | t2d-standard-2 | 6 | 2.07 | 4.69 | 0.90 | 0.45 (±55 %) | 1.39 (±39 %) | 0.66 (±33 %) | 0.86 (±14 %) | 0.66 (±34 %) |

## Vitesse par type de tâche (taille moyenne), relative à la référence

| VM | cpu_task | matrix_task | memory_task | sort_task |
|---|---|---|---|---|
| cloudopt-n-e2std4 | 1.03 | 1.07 | 1.07 | 1.03 |
| cloudopt-n-n2 | 1.29 | 1.30 | 1.35 | 1.30 |
| cloudopt-n-n2d | 4.66 | 1.69 | 1.57 | 1.43 |
| cloudopt-n-t2d | 4.69 | 1.76 | 1.59 | 1.43 |
