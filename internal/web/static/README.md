# Static assets — interface web Louvores-Go

Coloque aqui arquivos estáticos (main.css gerado pelo Tailwind, favicons, etc).

- O arquivo principal para Tailwind deve ser gerado como `main.css`.
- Configure o Tailwind para varrer também arquivos `.templ` no build.

Exemplo de build Tailwind (diretório raiz):

```bash
tailwindcss -i ./internal/web/static/input.css \
            -o ./internal/web/static/main.css \
            --content ./internal/web/templates/**/*.templ
```
