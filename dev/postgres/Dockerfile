FROM postgres:16

# Copy your schema files into the Postgres init directory
COPY init.sql /docker-entrypoint-initdb.d/

# Optional: Set permissions if needed
RUN chmod -R 755 /docker-entrypoint-initdb.d
