# Résumé de la conversation d'analyse (14/11/2026)

## Contexte

Projet de semestre ESILV A5 CCC « Cloud Optimization », en équipe de 3. La question posée : sur quelle VM GCP exécuter chaque tâche ?

La note (/20) est attribuée entièrement à la soutenance du TD6, pour 12 à 15 min de présentation suivies de 8 à 10 min de questions. Le barème compte :

| Bloc | Points |
|---|---|
| Cadrage | 4 |
| Exécution distribuée | 4 |
| Optimisation | 5 |
| Preuves et analyse | 4 |
| Soutenance | 3 |

Le prof exige des **mesures réelles**. Le Round-Robin (RR) sert de référence obligatoire, mais ne compte pas comme méthode d'optimisation. Le choix de la méthode est libre, à condition de la relier au cours. Les 3 membres doivent maîtriser tout le projet.

Le TD5 a lieu vendredi. Il faut y apporter :

- la fonction objectif ;
- l'état d'avancement ;
- des résultats préliminaires.

La validation finale a lieu environ une semaine après.

## Les 3 dossiers

| Dossier | Contenu | Statut |
|---|---|---|
| `~/Documents/ESILV/cloud/cloud-optimization` | **Base saine**, celle sur laquelle on travaille | Dépôt git et Makefile ; étapes 1 à 3 faites ; 4 VM benchmarkées, vm3 comprise ; contient `CLAUDE.md`, `docs/CONTEXTE_PROJET.md` (sujet, TD, cours, décisions) et les PDF dans `docs/` |
| `~/Documents/ESILV/cloud/Cloud_Optimization_V8_Projet_Complet_20261110_1809/cloud-optimization` | Projet du camarade, codé avec une IA, que l'utilisateur ne comprenait pas | Superflu rangé dans `archive/` ; sauvegarde intacte dans `cloud-optimization_SAUVEGARDE_originale` (dossier voisin) |
| `~/Documents/cloud-optimization` | Ancienne version des TD d'octobre, sans git | **À ne pas utiliser** : les tailles de tâches y sont tronquées (500 au lieu de 500000, 25 au lieu de 256), ce qui donne des tâches de 18 µs |

## Analyse du projet du camarade

**Ce qui est bien :**

- TD1 à TD4 sont corrects.
- `run_task.sh` et `run_all.sh` sont propres.
- `test_round_robin_20.sh` démarre les VM, envoie les fichiers, exécute, récupère les résultats et arrête les VM automatiquement (via un `trap`).
- La fonction `predict` (l.112) et la fonction `evaluate` (l.186) de `scheduler/optimization/weighted_greedy_v6.go` sont réutilisables.
- Les heuristiques MET, MCT, Min-Min et Max-Min sont codées dans `compare_algorithms.go`.

**Les problèmes :**

1. Il y a 8 versions empilées. Les 260 premières lignes de la V6 et de `compare_algorithms.go` sont identiques, copiées-collées.
2. Le glouton de la V6 traite les tâches dans l'ordre du fichier, sans tri (un « MCT avec score »). À cause de la facturation minimale de 60 s, il place tout sur la VM la moins chère : score 1,20, pire que le RR. C'est la recherche locale qui corrige ensuite.
3. La dernière étape de la V6 (l.594) change d'objectif : elle minimise uniquement le temps, avec un budget de coût de +10 %, et ne respecte donc plus l'objectif 60/40.
4. Au final, selon le modèle, la V6 coûte autant que le RR (0,00246 $) mais elle est plus rapide (2,27 s contre 3,08 s), pour un score de 0,895. **Rien n'a été mesuré en vrai.**
5. La comparaison avec les algorithmes du cours porte sur la V4, pas sur la V6.
6. vm3 n'a jamais été benchmarkée : ses valeurs sont une copie de vm2.
7. La seule mesure réelle est 1 run RR (environ 5,5 s).
8. Les tâches sont trop courtes (environ 0,2 s, alors qu'une connexion SSH prend 2 à 5 s).
9. Il n'y a pas de README pour TD4/TD5 ni de rapport.
10. `remote_batch_runner.py` fait tourner N workers par VM, c'est-à-dire une **file par VM**, ce qui est **interdit par le TD3 et le TD4**. Ces TD demandent un SSH indépendant par tâche avec `&` puis `wait`, toutes les tâches d'une VM tournant en même temps.
11. Aucun solveur n'a été utilisé : tout est heuristique.

## Décisions

**Objectif :**

```
min 0,6·Coût/Coût_RR + 0,4·Makespan/Makespan_RR
```

sous contraintes :

- chaque tâche est placée sur exactement une VM ;
- la RAM utilisée reste ≤ 85 % de celle de la VM ;
- le makespan reste ≤ 1,25 × makespan du RR.

Le coût d'une VM vaut prix × max(60 s, durée d'utilisation).

**Méthode principale :** un glouton clair (filtrer → noter → assigner, voir CM3 et CM6) que les 3 membres savent expliquer. Il est recommandé de trier les tâches de la plus longue à la plus courte (LPT).

**Bonus facultatifs :**

- une recherche locale (déplacements et échanges, effectués dans le calcul avant l'exécution) lancée depuis plusieurs heuristiques du cours (multi-départ, CM5) ;
- un ILP avec OR-Tools pour mesurer l'écart à l'optimum (CM4).

Rien de tout cela n'est obligatoire.

**Priorité absolue : les mesures réelles.**

- 3 runs RR et 3 runs optimisés, avec le même orchestrateur.
- Calculer le coût réel.
- Analyser l'écart entre prédit et mesuré.

**Avant de mesurer :**

1. Allonger les tâches pour qu'elles durent 10 à 30 s.
2. Refaire le benchmark des 4 VM avec ces nouvelles tailles.

**Règle :** ne jamais lancer d'action GCP sans accord explicite, car cela coûte de l'argent.

## Points ouverts dans la base saine

- `data/machines.csv` liste 3 VM, alors que highcpu a aussi été benchmarkée. Faut-il utiliser 3 ou 4 VM ?
- Le README prévoit un orchestrateur avec « une file par VM », ce qui contredit le TD3 et le TD4. Il faut garder `&` + `wait`, ou demander l'avis du prof au TD5.
- Les tâches sont toujours trop courtes (par exemple `cpu_task 500000` prend environ 0,15 s).

## Prochaines étapes

1. Orchestrateur conforme au TD3 (`&` + `wait`), puis exécution réelle du RR.
2. Scheduler glouton 60/40, qui pourra réutiliser `predict` et `evaluate` de l'ancien projet.
3. Choix des nouvelles tailles de tâches et nouveau benchmark des 4 VM.
4. 3 runs RR et 3 runs optimisés, puis analyse.
5. README, rapport, slides et démo.
