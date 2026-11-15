# Notes orales du CM6 (prof : Daniel Wladdimiro)

Résumé de ce que le prof a dit à l'oral pendant le CM6, en complément des slides (`docs/CM6.pdf`).

## Organisation

- Le CM6 a eu lieu avant le CM5. Le CM5 (Farah) est reporté à la semaine suivante.
- Le TD5, vendredi, est le dernier TD avec Daniel avant la validation finale. Il restera ensuite une semaine pour finir le projet.
- **À apporter vendredi** : la fonction objectif, l'état d'avancement et, si possible, des résultats préliminaires. C'est la dernière occasion de vérifier que l'approche est la bonne.

## Points clés

- **Le Round-Robin est la référence.** Notre méthode peut être meilleure sur un critère (le coût par exemple) et moins bonne sur un autre (le temps), à condition de le justifier. Le compromis doit rester raisonnable : « 1 h au lieu de 1 min », ce n'est pas défendable.
- **Énoncer clairement les hypothèses.** Exemple : « Mon modèle minimise le coût. Le déploiement sera moins cher, la latence un peu plus élevée. » Une hypothèse qui se révèle fausse est acceptable si on sait expliquer pourquoi. Une solution non optimale mais bien justifiée vaut mieux qu'une solution non justifiée.
- **Scheduling, placement et exécution sont trois choses distinctes.** Il faut savoir qui lance l'exécution et comment on détecte qu'une tâche est terminée.
- **Parallélisme** : deux tâches sur deux VM. **Concurrence** : deux tâches sur la même VM, qui se partagent le CPU et la RAM. La concurrence coûte moins cher mais va plus lentement. C'est un compromis entre coût et temps.
- **Le cycle de décision :**
  1. partir des tâches en attente ;
  2. **filtrer** les VM qui ne peuvent pas accueillir la tâche ;
  3. **noter** les VM restantes, par exemple avec un **score normalisé entre 0 et 1** qui reflète la fonction objectif ;
  4. assigner la tâche ;
  5. exécuter, observer, puis recommencer en tenant compte des tâches déjà en cours.
- **La faisabilité d'abord.** Le RR ne vérifie pas la capacité des VM : il peut dépasser le CPU disponible. Un filtre garantit que l'allocation peut réellement s'exécuter.
- **Le score dépend de l'objectif.** Pour répartir la charge, on choisit la VM la plus libre. Pour réduire le coût, on remplit une VM déjà utilisée et on laisse la VM vide éteinte.
- **Si aucune VM n'a assez de place**, deux options : ajouter une VM, ou laisser la tâche **en attente** jusqu'à ce que d'autres tâches se terminent. **« Running tasks in batches is a valid option for your project. »**
- **La théorie diffère de la réalité.** Les durées varient, les ressources sont partagées sur GCP, un démarrage peut prendre du retard, une tâche peut échouer. Il faut donc estimer, exécuter, mesurer, puis affiner le modèle.
  - Faire **au moins 3 runs** par expérience.
  - **Vérifier que les résultats sont corrects avant** de comparer la vitesse ou le coût.
- **Région** : certains groupes n'ont pas pu déployer dans la région de Paris, car le type de machine n'y était pas disponible.
- **Kubernetes** fonctionne selon la même logique : filtrer, noter, puis lier (*binding*). Les ressources déclarées ne correspondent pas toujours à la consommation réelle.
- **Notre projet est offline** : la liste des tâches est connue à l'avance.
- **Exercice final** (3 VM, 10 tâches) : finir vite coûte cher, minimiser la facture rend lent, et on peut aussi chercher à éviter la surcharge. Aucune allocation ne satisfait tous ces objectifs à la fois : c'est à nous de choisir.
