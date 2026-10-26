FROM python:3.13-alpine
WORKDIR /app
COPY scripts/fury_receiver.py /app/receiver.py
EXPOSE 8787
CMD ["python", "/app/receiver.py"]
