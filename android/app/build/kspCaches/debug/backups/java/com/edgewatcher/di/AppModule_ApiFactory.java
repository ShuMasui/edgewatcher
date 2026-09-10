package com.edgewatcher.di;

import com.edgewatcher.domain.port.ObservationApi;
import com.edgewatcher.infrastructure.api.EdgeWatcherService;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;
import kotlinx.serialization.json.Json;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_ApiFactory implements Factory<ObservationApi> {
  private final Provider<EdgeWatcherService> serviceProvider;

  private final Provider<Json> jsonProvider;

  private AppModule_ApiFactory(Provider<EdgeWatcherService> serviceProvider,
      Provider<Json> jsonProvider) {
    this.serviceProvider = serviceProvider;
    this.jsonProvider = jsonProvider;
  }

  @Override
  public ObservationApi get() {
    return api(serviceProvider.get(), jsonProvider.get());
  }

  public static AppModule_ApiFactory create(Provider<EdgeWatcherService> serviceProvider,
      Provider<Json> jsonProvider) {
    return new AppModule_ApiFactory(serviceProvider, jsonProvider);
  }

  public static ObservationApi api(EdgeWatcherService service, Json json) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.api(service, json));
  }
}
