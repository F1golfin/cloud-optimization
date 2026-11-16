# Rapport LaTeX

```
main.tex              préambule, page de garde, résumé, plan
sections/01..10.tex   une section par fichier
sections/annexe.tex   reproduction, organisation du dépôt, formats
```

Toutes les figures sont dessinées en LaTeX (TikZ / pgfplots) : aucune image à
importer. Les chiffres viennent de `docs/RESULTATS.md` et du dossier `results/`.

## Importer dans Overleaf

1. Créer l'archive (depuis la racine du dépôt) :
   ```bash
   cd docs && zip -r rapport.zip rapport -x '*/._*' && cd ..
   ```
2. Overleaf → **New Project** → **Upload Project** → choisir `docs/rapport.zip`.
3. Menu → **Compiler : pdfLaTeX**, document principal : `main.tex`.

Remplacer « Prénom NOM » sur la page de garde (`main.tex`).
