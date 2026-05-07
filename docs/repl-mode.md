# CLUE CODE — Mode REPL interactif

## Démarrage rapide

```bash
clue-code
```

Lance une session de chat interactive avec l'IA configurée.
Aucun argument requis. Le REPL maintient l'historique de la conversation jusqu'à la sortie.

## Exemple de session

```
CLUE CODE 0.1.0-dev — chat interactif
Modele : anthropic/claude-sonnet-4-5
Tape /help pour les commandes, /exit pour quitter

> bonjour
Claude: Bonjour ! Comment puis-je vous aider aujourd'hui ?

> écris une fonction Go qui inverse une chaîne
Claude: Voici une fonction Go pour inverser une chaîne :

    func reverse(s string) string {
        r := []rune(s)
        for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
            r[i], r[j] = r[j], r[i]
        }
        return string(r)
    }

> ajoute des tests unitaires
Claude: Bien sûr ! Voici les tests pour la fonction reverse...
```

## Commandes meta

| Commande | Description |
|----------|-------------|
| `/help` ou `/?` | Affiche la liste des commandes |
| `/exit` ou `/quit` | Quitte le REPL proprement |
| `/clear` | Efface l'historique de la conversation (nouveau sujet) |
| `/save <fichier>` | Sauvegarde la conversation en Markdown |
| `/model <id>` | Change le modèle pour les prochaines requêtes |
| `/tokens` | Affiche le coût total de la session en tokens |

### Exemples

```bash
# Changer de modèle en cours de session
> /model anthropic/claude-haiku-4-5

# Sauvegarder la conversation
> /save ~/conversations/debug-session.md

# Voir la consommation de tokens
> /tokens
[tokens] prompt=1240 completion=380 total=1620

# Effacer et repartir sur un nouveau sujet
> /clear
[history cleared]
```

## Saisie multi-ligne

Termine une ligne par `\` pour continuer sur la ligne suivante :

```
> Voici un long prompt \
... qui s'étend sur plusieurs \
... lignes
Claude: ...
```

## Raccourcis clavier

| Raccourci | Action |
|-----------|--------|
| `Ctrl+D` | Quitte le REPL (EOF) |
| `Ctrl+C` | Interrompt la réponse en cours (retourne au prompt) |

## Mode non-interactif (pipe / scripts)

Si stdin n'est pas un terminal (pipe ou redirection), le REPL passe automatiquement
en mode batch : lit tout stdin comme un seul prompt et quitte après la réponse.

```bash
# Équivalent à clue-code chat "..." mais via pipe
echo "résume ce texte" | clue-code

# Depuis un fichier
cat mon-fichier.txt | clue-code
```

## Sauvegarde Markdown

Le fichier généré par `/save` contient un frontmatter YAML suivi de la conversation :

```markdown
---
date: 2026-05-07T16:35:00Z
model: anthropic/claude-sonnet-4-5
tokens: 1240
---

## You
bonjour

## Claude
Bonjour ! Comment puis-je vous aider ?
```

## Comparaison one-shot vs REPL

| Critère | `clue-code chat "..."` | `clue-code` (REPL) |
|---------|----------------------|---------------------|
| Usage | Scripts, CI, pipes | Usage interactif |
| Historique | Non (single turn) | Oui (session entière) |
| Multi-tour | Non | Oui |
| Streaming | Oui | Oui |
| Commandes meta | Non | Oui (`/help`, `/save`…) |
| Exit | Automatique | `/exit` ou `Ctrl+D` |

## Variables d'environnement

| Variable | Effet |
|----------|-------|
| `NO_COLOR` | Désactive les couleurs ANSI (prompt et réponses en texte brut) |

## Installation et mise à jour

```bash
# Build local
go build -o ~/bin/clue-code ./cmd/clue-code

# Lancer le REPL
clue-code
```
