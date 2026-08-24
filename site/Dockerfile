FROM public.ecr.aws/docker/library/nginx:1.27-alpine

COPY nginx.conf /etc/nginx/nginx.conf
COPY dist/client /usr/share/nginx/html

RUN chown -R nginx:nginx /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O - http://127.0.0.1/healthz | grep -q "OK" || exit 1

CMD ["nginx", "-g", "daemon off;"]

LABEL org.opencontainers.image.title="agy-swap-site"
LABEL org.opencontainers.image.description="AGY Swap Landing Page"
