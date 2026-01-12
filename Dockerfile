# Use an official Python runtime as a parent image
FROM python:3.11-slim

# Set the working directory in the container
WORKDIR /app

# Install uv for package management
RUN pip install uv

# Copy the dependency files
COPY pyproject.toml uv.lock* ./

# Install dependencies using uv
# Using --system to install in the system python, as it's a container
# Using --no-cache to reduce image size
RUN uv pip install --system --no-cache -r pyproject.toml

# Copy the rest of the application code
COPY fsqd ./fsqd
COPY static ./static
COPY templates ./templates

# Make port 8080 available to the world outside this container
EXPOSE 8080

# Define environment variables with default values
ENV HOST=0.0.0.0
ENV PORT=8080
ENV DOWNLOAD_DIR=/downloads
ENV CACHE_DIR=/cache

# Create directories for downloads and cache
RUN mkdir -p /downloads /cache

# Run the application using the fsqd module
CMD ["python", "-m", "fsqd.main"]
